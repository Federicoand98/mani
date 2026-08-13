package fetch

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

func blockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func newSafeClient(blocked func(net.IP) bool, hostAllowed func(string) bool) *http.Client {
	d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolving %s: %w", host, err)
			}
			for _, ipa := range ips {
				if blocked(ipa.IP) {
					return nil, fmt.Errorf("refusing to connect to non-public address %s (%s)", ipa.IP, host)
				}
			}
			// Connette all'IP GIÀ verificato invece di lasciare che il dialer
			// risolva di nuovo il nome. Il certificato TLS resta verificato
			// contro l'hostname dell'URL, non contro l'IP: http.Transport prende
			// il ServerName da req.URL, non dall'indirizzo passato a DialContext.
			return d.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   requestTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects (%d)", len(via))
			}
			if s := req.URL.Scheme; s != "http" && s != "https" {
				return fmt.Errorf("refusing redirect to scheme %q", s)
			}
			if hostAllowed != nil && !hostAllowed(req.URL.Hostname()) {
				return fmt.Errorf("refusing redirect to %q: not allowed by policy.network", req.URL.Hostname())
			}
			return nil
		},
	}
}

var (
	reComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	reDrop    = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>` +
		`|<style\b[^>]*>.*?</style>` +
		`|<noscript\b[^>]*>.*?</noscript>` +
		`|<svg\b[^>]*>.*?</svg>` +
		`|<head\b[^>]*>.*?</head>`)
	reTitle      = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title>`)
	reBreak      = regexp.MustCompile(`(?i)</?(br|p|div|li|tr|h[1-6]|section|article|ul|ol|table)\b[^>]*>`)
	reTag        = regexp.MustCompile(`(?s)<[^>]*>`)
	reSpaces     = regexp.MustCompile(`[ \t\x{00a0}]+`)
	reBlankLines = regexp.MustCompile(`\n{3,}`)
)

func htmlToText(s string) string {
	title := ""
	if m := reTitle.FindStringSubmatch(s); m != nil {
		title = strings.TrimSpace(html.UnescapeString(stripTags(m[1])))
	}

	s = reComment.ReplaceAllString(s, "")
	s = reDrop.ReplaceAllString(s, "")
	s = reBreak.ReplaceAllString(s, "\n")
	s = stripTags(s)
	s = html.UnescapeString(s)

	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = reSpaces.ReplaceAllString(s, " ")

	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	s = reBlankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	s = strings.TrimSpace(s)

	if title != "" {
		return "# " + title + "\n\n" + s
	}
	return s
}

func stripTags(s string) string { return reTag.ReplaceAllString(s, "") }
