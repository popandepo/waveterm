//go:build windows

// conptyprobe runs a command in a pseudo console opened by the pty module that wavesrv uses, and reports what came out, as one JSON line.
// It exists to test the pty layer without Electron: which private modes (alternate screen, mouse tracking) reach the reader, whether input
// (a mouse click written as bytes) reaches the program, and that a resize, a shell and Unicode work.
//
//	conptyprobe [-cols 120] [-rows 30] [-wait 6000] [-send "text with \\x1b escapes" -send-after 1500] [-resize 100x40 -resize-after 1500] [-raw file] -- command args...
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

type result struct {
	Error  string          `json:"error,omitempty"`
	Bytes  int             `json:"bytes"`
	Modes  map[string]bool `json:"modes"`
	Text   string          `json:"text"`
	Exited bool            `json:"exited"`
}

func main() {
	cols := flag.Int("cols", 120, "columns")
	rows := flag.Int("rows", 30, "rows")
	wait := flag.Int("wait", 6000, "milliseconds to read before stopping")
	send := flag.String("send", "", "text written to the pty (Go string escapes: \\x1b, \\r)")
	sendAfter := flag.Int("send-after", 1500, "milliseconds before -send")
	resize := flag.String("resize", "", "COLSxROWS to resize to")
	resizeAfter := flag.Int("resize-after", 1500, "milliseconds before -resize")
	raw := flag.String("raw", "", "file that gets the raw bytes read")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: conptyprobe [flags] -- command args...")
		os.Exit(2)
	}
	res := result{Modes: map[string]bool{}}
	out := func() {
		b, _ := json.Marshal(res)
		fmt.Println(string(b))
	}

	cmd := exec.Command(args[0], args[1:]...)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(*rows), Cols: uint16(*cols)})
	if err != nil {
		res.Error = err.Error()
		out()
		os.Exit(1)
	}
	defer ptmx.Close()

	var mu sync.Mutex
	var buf bytes.Buffer
	go func() {
		chunk := make([]byte, 65536)
		for {
			n, err := ptmx.Read(chunk)
			if n > 0 {
				mu.Lock()
				buf.Write(chunk[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()

	if *send != "" {
		time.AfterFunc(time.Duration(*sendAfter)*time.Millisecond, func() {
			s, err := strconv.Unquote(`"` + strings.ReplaceAll(*send, `"`, `\"`) + `"`)
			if err != nil {
				s = *send
			}
			_, _ = ptmx.Write([]byte(s))
		})
	}
	if *resize != "" {
		var c, r int
		if _, err := fmt.Sscanf(*resize, "%dx%d", &c, &r); err == nil {
			time.AfterFunc(time.Duration(*resizeAfter)*time.Millisecond, func() {
				_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(r), Cols: uint16(c)})
			})
		}
	}

	select {
	case <-done:
		res.Exited = true
		time.Sleep(500 * time.Millisecond) // let the last output through
	case <-time.After(time.Duration(*wait) * time.Millisecond):
	}
	if cmd.Process != nil && !res.Exited {
		_ = cmd.Process.Kill()
	}
	mu.Lock()
	data := append([]byte(nil), buf.Bytes()...)
	mu.Unlock()
	res.Bytes = len(data)
	for _, m := range regexp.MustCompile(`\x1b\[\?(\d+)([hl])`).FindAllSubmatch(data, -1) {
		res.Modes[string(m[1])+string(m[2])] = true
	}
	res.Text = strconv.QuoteToASCII(string(data))
	if len(res.Text) > 6000 {
		res.Text = res.Text[:6000] + "..."
	}
	if *raw != "" {
		_ = os.WriteFile(*raw, data, 0o600)
	}
	out()
}
