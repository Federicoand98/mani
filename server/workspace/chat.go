package workspace

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/Federicoand98/mani/app"
	"github.com/Federicoand98/mani/config"
	"github.com/Federicoand98/mani/core"
	"github.com/Federicoand98/mani/server/wire"
	"github.com/Federicoand98/mani/session"
)

// Limits on what one message may carry.
const (
	maxImages     = 4
	maxImageBytes = 5 << 20
	defaultKeep   = 20
	minKeep       = 2
	maxKeep       = 500
)

var imageTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

// chats owns the conversations of the working directory and the replies being
// written. A reply runs on the server's context, not the connection's: closing
// the tab does not stop it, and it is saved when it ends (decision M2-4).
type chats struct {
	dir string // <workdir>/.mani/sessions

	once     sync.Once
	store    session.Store
	storeErr error

	mu    sync.Mutex
	turns map[string]*turn

	ctx  context.Context // cancelled when mani web stops
	stop context.CancelFunc
	wg   sync.WaitGroup

	// newRuntime builds the model-only runtime for a reply; tests swap it.
	newRuntime func(cfg config.Config, store session.Store, keep int) (*app.Runtime, error)
}

type turn struct {
	cancel context.CancelFunc
	mu     sync.Mutex
	subs   map[*subscriber]bool
}

type subscriber struct {
	ctx context.Context
	ch  chan wire.ServerMsg
}

func newChats(workdir string) *chats {
	ctx, stop := context.WithCancel(context.Background())
	return &chats{
		dir:        filepath.Join(workdir, ".mani", "sessions"),
		turns:      map[string]*turn{},
		ctx:        ctx,
		stop:       stop,
		newRuntime: modelRuntime,
	}
}

// modelRuntime is a runtime with no tools: in M2 the chat is the model alone.
func modelRuntime(cfg config.Config, store session.Store, keep int) (*app.Runtime, error) {
	rt := app.NewFromConfig(cfg).WithSessionStore(store)
	if err := rt.ClientErr(); err != nil {
		return nil, err
	}
	app.RegisterTrimCompaction(rt, keep)
	return rt, nil
}

// sessions opens the store on first use. The folder gets its own .gitignore,
// so conversations stay out of a project's history without touching its files.
func (c *chats) sessions() (session.Store, error) {
	c.once.Do(func() {
		root := filepath.Dir(c.dir)
		if err := os.MkdirAll(c.dir, 0o755); err != nil {
			c.storeErr = fmt.Errorf("conversations: %w", err)
			return
		}
		ignore := filepath.Join(root, ".gitignore")
		if _, err := os.Stat(ignore); errors.Is(err, os.ErrNotExist) {
			_ = os.WriteFile(ignore, []byte("# written by mani web: conversations are not part of the project\n*\n"), 0o644)
		}
		c.store, c.storeErr = session.NewFileStore(c.dir)
	})
	return c.store, c.storeErr
}

func (c *chats) running(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.turns[id] != nil
}

// close stops the replies in progress; each one saves what it has.
func (c *chats) close() {
	c.stop()
	c.wg.Wait()
}

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	st, err := s.chats.sessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	metas, err := st.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := ConversationList{Conversations: []ConversationMeta{}}
	for _, m := range metas {
		out.Conversations = append(out.Conversations, ConversationMeta{
			ID: m.ID, Title: m.Title, Provider: m.Provider, Model: m.Model,
			UpdatedAt: m.UpdatedAt.UTC().Format(time.RFC3339), Running: s.chats.running(m.ID),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	var in ConversationInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body: "+err.Error())
		return
	}
	if msg := checkModel(in.Provider, in.Model); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	st, err := s.chats.sessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sess := session.New(in.Model)
	sess.Provider = in.Provider
	sess.Title = in.Title
	if err := st.Save(sess); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, s.chats.toDTO(sess))
}

// checkModel refuses a provider that is unknown or not added.
func checkModel(provider, model string) string {
	if provider == "" || model == "" {
		return "a conversation needs a provider and a model"
	}
	e, ok := lookup(provider)
	if !ok {
		return fmt.Sprintf("unknown provider %q", provider)
	}
	cfg, err := config.Load()
	if err != nil {
		return err.Error()
	}
	if !cfg.Providers[provider].Enabled {
		return e.label + " is not added: add it in Providers first"
	}
	return ""
}

// conversation loads {id} or answers 404.
func (s *Server) conversation(w http.ResponseWriter, r *http.Request) (*session.Session, session.Store, bool) {
	st, err := s.chats.sessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return nil, nil, false
	}
	sess, err := st.Load(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "no such conversation")
		return nil, nil, false
	}
	return sess, st, true
}

func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	sess, _, ok := s.conversation(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.chats.toDTO(sess))
}

func (s *Server) handlePatchConversation(w http.ResponseWriter, r *http.Request) {
	var in ConversationInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body: "+err.Error())
		return
	}
	sess, st, ok := s.conversation(w, r)
	if !ok {
		return
	}
	if s.chats.running(sess.ID) {
		writeError(w, http.StatusConflict, "the model is still answering in this conversation")
		return
	}
	if in.Provider != "" || in.Model != "" {
		p, m := pick(in.Provider, sess.Provider), pick(in.Model, sess.Model)
		if msg := checkModel(p, m); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		sess.Provider, sess.Model = p, m
	}
	if in.Title != "" {
		sess.Title = in.Title
	}
	if err := st.Save(sess); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.chats.toDTO(sess))
}

func pick(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	sess, st, ok := s.conversation(w, r)
	if !ok {
		return
	}
	if s.chats.running(sess.ID) {
		writeError(w, http.StatusConflict, "the model is still answering: stop it first")
		return
	}
	if err := st.Delete(sess.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *chats) toDTO(sess *session.Session) Conversation {
	out := Conversation{
		ConversationMeta: ConversationMeta{
			ID: sess.ID, Title: sess.Title, Provider: sess.Provider, Model: sess.Model,
			UpdatedAt: sess.UpdatedAt.UTC().Format(time.RFC3339), Running: c.running(sess.ID),
		},
		Messages: []ChatMessage{},
	}
	for _, m := range sess.Memory().Messages() {
		if m.Role != core.RoleUser && m.Role != core.RoleAssistant {
			continue
		}
		msg := ChatMessage{Role: string(m.Role), Parts: []ChatPart{}}
		for _, b := range m.Content {
			switch v := b.(type) {
			case core.TextBlock:
				msg.Parts = append(msg.Parts, ChatPart{Type: "text", Text: v.Text})
			case core.ImageBlock:
				msg.Parts = append(msg.Parts, ChatPart{Type: "image", MediaType: v.MediaType, Data: base64.StdEncoding.EncodeToString(v.Data)})
			}
		}
		if len(msg.Parts) > 0 {
			out.Messages = append(out.Messages, msg)
		}
	}
	return out
}

// handleChatSocket carries one conversation: input and cancel in, the reply out.
// The cookie was checked by s.api; websocket.Accept refuses an Origin that is
// not this host, which is what stops another site from opening it.
func (s *Server) handleChatSocket(w http.ResponseWriter, r *http.Request) {
	sess, st, ok := s.conversation(w, r)
	if !ok {
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxImages*maxImageBytes*4/3 + 1<<20) // base64 images plus the text

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	sub := &subscriber{ctx: ctx, ch: make(chan wire.ServerMsg, 64)}

	go func() { // the only writer on this connection
		for {
			select {
			case <-ctx.Done():
				return
			case m := <-sub.ch:
				if err := wsjson.Write(ctx, conn, m); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	s.chats.attach(sess.ID, sub) // a reply already running: tell the page

	for {
		var msg wire.ClientMsg
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			s.chats.detach(sess.ID, sub)
			return
		}
		switch msg.Type {
		case "input":
			if err := s.chats.start(sess.ID, st, msg, sub); err != nil {
				sub.send(wire.ErrorMsg(err.Error()))
			}
		case "cancel":
			s.chats.cancel(sess.ID)
		default:
			sub.send(wire.ErrorMsg(fmt.Sprintf("unknown message type %q", msg.Type)))
		}
	}
}

func (sub *subscriber) send(m wire.ServerMsg) {
	select {
	case sub.ch <- m:
	case <-sub.ctx.Done():
	}
}

func (c *chats) attach(id string, sub *subscriber) {
	c.mu.Lock()
	t := c.turns[id]
	c.mu.Unlock()
	if t == nil {
		return
	}
	t.mu.Lock()
	t.subs[sub] = true
	t.mu.Unlock()
	sub.send(wire.ServerMsg{Type: "busy"})
}

func (c *chats) detach(id string, sub *subscriber) {
	c.mu.Lock()
	t := c.turns[id]
	c.mu.Unlock()
	if t != nil {
		t.mu.Lock()
		delete(t.subs, sub)
		t.mu.Unlock()
	}
}

func (c *chats) cancel(id string) {
	c.mu.Lock()
	t := c.turns[id]
	c.mu.Unlock()
	if t != nil {
		t.cancel()
	}
}

// start validates a message and begins the reply. It reloads the conversation
// from disk, so a model changed with PATCH since the page opened is honoured.
func (c *chats) start(id string, st session.Store, msg wire.ClientMsg, sub *subscriber) error {
	images, err := decodeImages(msg.Images)
	if err != nil {
		return err
	}
	if msg.Input == "" && len(images) == 0 {
		return errors.New("empty message")
	}
	keep := msg.Keep
	if keep == 0 {
		keep = defaultKeep
	}
	if keep < minKeep || keep > maxKeep {
		return fmt.Errorf("memory must be between %d and %d messages, got %d", minKeep, maxKeep, keep)
	}

	c.mu.Lock()
	if c.turns[id] != nil {
		c.mu.Unlock()
		return errors.New("the model is still answering: wait for it, or stop it")
	}
	sess, err := st.Load(id)
	if err != nil {
		c.mu.Unlock()
		return errors.New("no such conversation")
	}
	if msg := checkModel(sess.Provider, sess.Model); msg != "" {
		c.mu.Unlock()
		return errors.New(msg)
	}
	cfg, err := config.Load()
	if err != nil {
		c.mu.Unlock()
		return err
	}
	cfg.Provider = sess.Provider
	cfg.SetActiveModel(sess.Model)
	rt, err := c.newRuntime(cfg, st, keep)
	if err != nil {
		c.mu.Unlock()
		return err
	}

	ctx, cancel := context.WithCancel(app.WithSource(c.ctx, "web"))
	t := &turn{cancel: cancel, subs: map[*subscriber]bool{sub: true}}
	c.turns[id] = t
	c.wg.Add(1)
	c.mu.Unlock()

	attachments := make([]core.ContentBlock, len(images))
	for i, img := range images {
		attachments[i] = img
	}
	events, _ := rt.ExecuteIn(ctx, sess, msg.Input, attachments...)

	go func() {
		defer c.wg.Done()
		defer cancel()
		var last wire.ServerMsg
		for ev := range events {
			if m, ok := wire.FromEvent(ev); ok {
				last = m
				if m.Type == "done" || m.Type == "error" || m.Type == "cancelled" {
					continue // sent once the conversation is saved
				}
				t.broadcast(m)
			}
		}
		// ExecuteIn saves on success and on cancel, not on error: save always,
		// so the question survives a provider that refused to answer.
		if err := st.Save(sess); err != nil && last.Type != "error" {
			last = wire.ErrorMsg("the reply was not saved: " + err.Error())
		}
		c.mu.Lock()
		delete(c.turns, id)
		c.mu.Unlock()
		t.broadcast(last)
	}()
	return nil
}

func (t *turn) broadcast(m wire.ServerMsg) {
	t.mu.Lock()
	subs := make([]*subscriber, 0, len(t.subs))
	for s := range t.subs {
		subs = append(subs, s)
	}
	t.mu.Unlock()
	for _, s := range subs {
		s.send(m)
	}
}

func decodeImages(in []wire.Image) ([]core.ImageBlock, error) {
	if len(in) > maxImages {
		return nil, fmt.Errorf("at most %d images per message, got %d", maxImages, len(in))
	}
	out := make([]core.ImageBlock, 0, len(in))
	for i, img := range in {
		if !slices.Contains(imageTypes, img.MediaType) {
			return nil, fmt.Errorf("image %d: %q is not supported (png, jpeg, gif or webp)", i+1, img.MediaType)
		}
		data, err := base64.StdEncoding.DecodeString(img.Data)
		if err != nil {
			return nil, fmt.Errorf("image %d: not valid base64", i+1)
		}
		if len(data) > maxImageBytes {
			return nil, fmt.Errorf("image %d is %.1f MB: the limit is %d MB", i+1, float64(len(data))/(1<<20), maxImageBytes>>20)
		}
		out = append(out, core.ImageBlock{MediaType: img.MediaType, Data: data})
	}
	return out, nil
}
