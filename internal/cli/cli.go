// Package cli implements the remote Lumio client without server dependencies.
package cli

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"
)

const Help = `Usage: lumio-cli [--url URL] [--config FILE] [--json] COMMAND [OPTIONS]

Commands:
  version                   Print client build version
  login [--password-stdin]   Sign in; otherwise reads LUMIO_PASSWORD
  logout                    Revoke and remove the saved session
  status                    Library and storage status
  list [--all] [--query TEXT] [--favorite] [--from RFC3339] [--until RFC3339]
       [--cursor CURSOR]     List photos/videos (one page unless --all)
  show ID                   Photo/video metadata
  upload FILE...            Upload photos/videos sequentially
  download --output FILE ID Download an original without overwriting files
  favorite [--unset] ID      Set or clear a favorite
  delete --yes ID...         Delete selected photos/videos
  help                      Show this help

Global flags precede COMMAND. LUMIO_URL defaults to http://localhost:8080.
Sessions are stored in the user config directory under lumio/cli.json;
LUMIO_CLI_CONFIG or --config overrides it. Passwords are never saved.
Use --json for scriptable list output. Other metadata responses are JSON.
`

type session struct {
	URL    string       `json:"url"`
	Cookie *http.Cookie `json:"cookie"`
}

type client struct {
	ctx         context.Context
	base        *url.URL
	http        *http.Client
	out, errOut io.Writer
	json        bool
}

// Run executes a remote client command.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("lumio-cli", flag.ContinueOnError)
	fs.SetOutput(errOut)
	endpoint := fs.String("url", os.Getenv("LUMIO_URL"), "Lumio server URL")
	config := fs.String("config", os.Getenv("LUMIO_CLI_CONFIG"), "session file")
	asJSON := fs.Bool("json", false, "JSON output")
	fs.Usage = func() { fmt.Fprint(errOut, Help) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	args = fs.Args()
	if len(args) == 0 {
		return errors.New("provide a command; use lumio-cli help")
	}
	if args[0] == "help" {
		_, err := fmt.Fprint(out, Help)
		return err
	}
	if *config == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		*config = filepath.Join(dir, "lumio", "cli.json")
	}
	var saved session
	raw, err := os.ReadFile(*config)
	if err == nil {
		if err := json.Unmarshal(raw, &saved); err != nil {
			return fmt.Errorf("read session: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if *endpoint == "" {
		*endpoint = saved.URL
	}
	if *endpoint == "" {
		*endpoint = "http://localhost:8080"
	}
	base, err := url.Parse(strings.TrimRight(*endpoint, "/"))
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return errors.New("--url must be an http(s) server URL without credentials, query or fragment")
	}
	jar, _ := cookiejar.New(nil)
	if saved.URL == base.String() && saved.Cookie != nil {
		jar.SetCookies(base, []*http.Cookie{saved.Cookie})
	}
	c := &client{ctx: ctx, base: base, http: &http.Client{Jar: jar, Timeout: 10 * time.Minute, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, out: out, errOut: errOut, json: *asJSON}
	command := args[0]
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	f.SetOutput(errOut)
	var stdinPassword, all, favorite, unset, yes bool
	var query, from, until, cursor, output string
	switch command {
	case "login":
		f.BoolVar(&stdinPassword, "password-stdin", false, "read password from standard input")
	case "list":
		f.BoolVar(&all, "all", false, "fetch every page")
		f.BoolVar(&favorite, "favorite", false, "favorites only")
		f.StringVar(&query, "query", "", "filename search")
		f.StringVar(&from, "from", "", "inclusive upload date")
		f.StringVar(&until, "until", "", "exclusive upload date")
		f.StringVar(&cursor, "cursor", "", "page cursor")
	case "favorite":
		f.BoolVar(&unset, "unset", false, "clear favorite")
	case "delete":
		f.BoolVar(&yes, "yes", false, "confirm deletion")
	case "download":
		f.StringVar(&output, "output", "", "destination file (required)")
	case "logout", "status", "show", "upload":
	default:
		return fmt.Errorf("unknown command %q; use lumio-cli help", command)
	}
	if err := f.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	rest := f.Args()
	switch command {
	case "show", "download", "favorite":
		if len(rest) != 1 {
			return fmt.Errorf("%s requires one photo ID", command)
		}
	case "upload", "delete":
		if len(rest) == 0 {
			return fmt.Errorf("%s requires at least one %s", command, map[string]string{"upload": "file", "delete": "photo ID"}[command])
		}
	default:
		if len(rest) != 0 {
			return fmt.Errorf("unexpected argument %q", rest[0])
		}
	}
	if command == "show" || command == "download" || command == "favorite" || command == "delete" {
		for _, id := range rest {
			if len(id) != 64 || id != strings.ToLower(id) {
				return errors.New("photo IDs must be lowercase SHA-256 hashes")
			}
			if _, err := hex.DecodeString(id); err != nil {
				return errors.New("invalid photo ID")
			}
		}
	}
	switch command {
	case "login":
		password := os.Getenv("LUMIO_PASSWORD")
		if stdinPassword {
			b, err := io.ReadAll(io.LimitReader(in, 8193))
			if err != nil {
				return err
			}
			if len(b) > 8192 {
				return errors.New("password input is too long")
			}
			password = strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r")
		}
		if password == "" {
			return errors.New("use --password-stdin or set LUMIO_PASSWORD")
		}
		resp, err := c.request("POST", "/api/v1/sessions", map[string]string{"password": password})
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		var cookie *http.Cookie
		for _, candidate := range resp.Cookies() {
			if candidate.Name == "lumio_session" {
				cookie = candidate
			}
		}
		if cookie == nil {
			return errors.New("server did not return a session cookie")
		}
		if cookie.Secure && base.Scheme != "https" {
			return errors.New("server requires HTTPS; use HTTPS or configure LUMIO_SECURE_COOKIE=false for local HTTP")
		}
		// Host-only cookies must stay scoped to the exact selected server.
		cookie.Domain = ""
		cookie.Path = "/"
		if err := saveSession(*config, session{URL: base.String(), Cookie: cookie}); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "Signed in to", base)
		return err
	case "logout":
		resp, err := c.request("DELETE", "/api/v1/sessions/current", nil)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if saved.URL == base.String() {
			if err := os.Remove(*config); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		_, err = fmt.Fprintln(out, "Signed out")
		return err
	case "status":
		return c.printJSON("GET", "/api/v1/status", nil)
	case "show":
		return c.printJSON("GET", "/api/v1/photos/"+rest[0], nil)
	case "favorite":
		return c.printJSON("PATCH", "/api/v1/photos/"+rest[0], map[string]bool{"favorite": !unset})
	case "delete":
		if !yes {
			return errors.New("deletion requires --yes; originals will be removed from the server")
		}
		for _, id := range rest {
			resp, err := c.request("DELETE", "/api/v1/photos/"+id, nil)
			if err != nil {
				return err
			}
			resp.Body.Close()
			if _, err = fmt.Fprintln(out, id); err != nil {
				return err
			}
		}
		return nil
	case "download":
		if output == "" {
			return errors.New("download requires --output FILE")
		}
		return c.download(rest[0], output)
	case "upload":
		for _, name := range rest {
			if err := c.upload(name); err != nil {
				return fmt.Errorf("upload %s: %w", name, err)
			}
		}
		return nil
	case "list":
		q := url.Values{}
		if len(query) > 200 {
			return errors.New("query must be at most 200 bytes")
		}
		for key, value := range map[string]string{"from": from, "until": until} {
			if value != "" {
				if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
					return fmt.Errorf("%s must be RFC3339", key)
				}
				q.Set(key, value)
			}
		}
		q.Set("q", query)
		if favorite {
			q.Set("favorite", "true")
		}
		q.Set("cursor", cursor)
		return c.list(q, all)
	}
	return nil
}

func saveSession(name string, s session) error {
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(name), ".lumio-session-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = json.NewEncoder(f).Encode(s); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), name)
}

func (c *client) request(method, path string, value any) (*http.Response, error) {
	var body io.Reader
	if value != nil {
		b, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	return c.send(method, path, body, "application/json")
}
func (c *client) send(method, path string, body io.Reader, typ string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(c.ctx, method, c.base.String()+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Lumio-Request", "1")
	if body != nil {
		req.Header.Set("Content-Type", typ)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		var problem struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&problem)
		if problem.Error == "" {
			problem.Error = resp.Status
		}
		if resp.StatusCode == 401 {
			problem.Error += "; run lumio-cli login"
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, problem.Error)
	}
	return resp, nil
}
func (c *client) printJSON(method, path string, body any) error {
	resp, err := c.request(method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var value json.RawMessage
	if err = json.NewDecoder(resp.Body).Decode(&value); err != nil {
		return err
	}
	enc := json.NewEncoder(c.out)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

type photo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Size     int64  `json:"size"`
	Favorite bool   `json:"favorite"`
}

func (c *client) list(q url.Values, all bool) error {
	// Emit pages as NDJSON with --json so large libraries need no aggregation.
	table := tabwriter.NewWriter(c.out, 0, 4, 2, ' ', 0)
	if !c.json {
		fmt.Fprintln(table, "ID\tNAME\tTYPE\tBYTES\tFAVORITE")
	}
	seen := map[string]bool{q.Get("cursor"): true}
	for {
		resp, err := c.request("GET", "/api/v1/photos?"+q.Encode(), nil)
		if err != nil {
			return err
		}
		var page struct {
			Photos     []json.RawMessage `json:"photos"`
			NextCursor string            `json:"nextCursor"`
		}
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return err
		}
		for _, raw := range page.Photos {
			if c.json {
				if _, err = fmt.Fprintln(c.out, string(raw)); err != nil {
					return err
				}
			} else {
				var p photo
				if err = json.Unmarshal(raw, &p); err != nil {
					return err
				}
				fmt.Fprintf(table, "%s\t%s\t%s\t%d\t%t\n", p.ID, safeText(p.Name), p.Type, p.Size, p.Favorite)
			}
		}
		if !all || page.NextCursor == "" {
			if !c.json {
				if err = table.Flush(); err != nil {
					return err
				}
			}
			if !all && page.NextCursor != "" {
				_, err = fmt.Fprintln(c.errOut, "Next cursor:", page.NextCursor)
			}
			return err
		}
		if seen[page.NextCursor] {
			return errors.New("server repeated a pagination cursor")
		}
		seen[page.NextCursor] = true
		q.Set("cursor", page.NextCursor)
	}
}
func safeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, s)
}

func (c *client) upload(name string) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("expected a regular file")
	}
	// Stream the multipart body instead of holding the photo/video in memory.
	reader, writer := io.Pipe()
	defer reader.Close()
	multipartWriter := multipart.NewWriter(writer)
	done := make(chan error, 1)
	go func() {
		part, err := multipartWriter.CreateFormFile("photo", filepath.Base(name))
		if err == nil {
			_, err = io.Copy(part, file)
		}
		if err == nil {
			err = multipartWriter.Close()
		}
		writer.CloseWithError(err)
		done <- err
	}()
	resp, requestErr := c.send("POST", "/api/v1/photos", reader, multipartWriter.FormDataContentType())
	// A server may reject the request before consuming the body.
	reader.Close()
	writeErr := <-done
	if requestErr != nil {
		return requestErr
	}
	defer resp.Body.Close()
	if writeErr != nil {
		return writeErr
	}
	var result json.RawMessage
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	return json.NewEncoder(c.out).Encode(result)
}
func (c *client) download(id, output string) error {
	if _, err := os.Lstat(output); err == nil {
		return fmt.Errorf("destination already exists: %s", output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	resp, err := c.request("GET", "/api/v1/photos/"+id+"/original?download=1", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.CreateTemp(filepath.Dir(output), ".lumio-download-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = io.Copy(f, resp.Body)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	// Link publishes the complete file atomically and refuses to overwrite.
	if err = os.Link(f.Name(), output); err != nil {
		return err
	}
	_, err = fmt.Fprintln(c.out, output)
	return err
}
