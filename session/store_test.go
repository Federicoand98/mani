package session

import (
	"sync"
	"testing"
)

// Every run of a Runtime saves into the same store, and runs are concurrent:
// triggers, server clients, batch jobs. Under -race this fails without the lock.
func TestInMemoryStore_ConcurrentUse(t *testing.T) {
	st := NewInMemoryStore()
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			s := New("test-model")
			_ = st.Save(s)
			_, _ = st.Load(s.ID)
			_, _ = st.List()
			_ = st.Delete(s.ID)
		})
	}
	wg.Wait()
}
