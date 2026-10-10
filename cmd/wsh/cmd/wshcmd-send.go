// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

const (
	SendKeyPrefix  = "key:"
	SendTextPrefix = "text:"
)

var sendCmd = &cobra.Command{
	Use:   "send [-b block] <token>...",
	Short: "Send text and named keys to the terminal of one block",
	Long: `Send input to the terminal of ONE block, by block reference (-b), whether or not that block, its tab or the
Wave window has focus. Nothing is typed into the window and the active tab is not changed: the bytes go through the
block's controller straight to the program running in it.

Each argument is one token and is written separately, with a short pause between tokens (--delay), because many
interactive programs read several keys arriving in one chunk as a paste. A token is:
  key:<Name>   a named key: Enter, Escape, Tab, BackTab, Backspace, Delete, Space, Up, Down, Left, Right, Home,
               End, PageUp, PageDown, Ctrl+<letter> (Ctrl+C sends the interrupt byte)
  text:<text>  literal text, sent as is (no Enter is added)
  <text>       anything else is literal text too
--stdin reads more literal text from standard input and sends it last, as one token (use it for text that is awkward
to put on a command line).

Text is plain text: control characters (ESC, newline, carriage return, backspace and the rest) and invisible format
characters are refused in text tokens, because they would act as keys. Use key:<Name> for keys. --raw lifts this for
text you really mean to send as bytes.`,
	Example: `  wsh send -b 3 key:Down key:Enter
  wsh send -b 3f2a9c10 "text:continue please" key:Enter
  wsh send -b 3 key:Ctrl+C`,
	RunE:    sendRun,
	PreRunE: preRunSetupRpcClient,
}

var (
	sendDelayMs int
	sendStdin   bool
	sendRaw     bool
)

func init() {
	sendCmd.Flags().IntVar(&sendDelayMs, "delay", 40, "pause in milliseconds between tokens")
	sendCmd.Flags().BoolVar(&sendStdin, "stdin", false, "also send standard input as literal text, last")
	sendCmd.Flags().BoolVar(&sendRaw, "raw", false, "allow control characters in text tokens (they will act as keys)")
	rootCmd.AddCommand(sendCmd)
}

var namedKeys = map[string]string{
	"enter":     "\r",
	"return":    "\r",
	"escape":    "\x1b",
	"esc":       "\x1b",
	"tab":       "\t",
	"backtab":   "\x1b[Z",
	"backspace": "\x7f",
	"delete":    "\x1b[3~",
	"space":     " ",
	"up":        "\x1b[A",
	"down":      "\x1b[B",
	"right":     "\x1b[C",
	"left":      "\x1b[D",
	"home":      "\x1b[H",
	"end":       "\x1b[F",
	"pageup":    "\x1b[5~",
	"pagedown":  "\x1b[6~",
}

func encodeKey(name string) (string, error) {
	lower := strings.ToLower(strings.TrimSpace(name))
	if val, ok := namedKeys[lower]; ok {
		return val, nil
	}
	if strings.HasPrefix(lower, "ctrl+") {
		rest := lower[len("ctrl+"):]
		if len(rest) == 1 && rest[0] >= 'a' && rest[0] <= 'z' {
			return string([]byte{rest[0] - 'a' + 1}), nil
		}
	}
	return "", fmt.Errorf("unknown key %q", name)
}

// validateTextToken refuses control and invisible format characters in plain text, so text cannot act as keys.
func validateTextToken(text string) error {
	for _, r := range text {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return fmt.Errorf("text contains the control or invisible character U+%04X; use key:<Name> for keys, or --raw to send it as bytes", r)
		}
	}
	return nil
}

// encodeSendTokens turns tokens into the chunks that are written one at a time.
func encodeSendTokens(tokens []string, raw bool) ([]string, error) {
	chunks := make([]string, 0, len(tokens))
	for i, tok := range tokens {
		switch {
		case strings.HasPrefix(tok, SendKeyPrefix):
			val, err := encodeKey(tok[len(SendKeyPrefix):])
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, val)
		default:
			text := tok
			if strings.HasPrefix(tok, SendTextPrefix) {
				text = tok[len(SendTextPrefix):]
			}
			if !raw {
				if err := validateTextToken(text); err != nil {
					return nil, fmt.Errorf("token %d: %w", i+1, err)
				}
			}
			chunks = append(chunks, text)
		}
	}
	return chunks, nil
}

type inputSender func(blockId string, data []byte) error

// deliverChunks writes each non-empty chunk to the block, pausing between chunks. It returns how many chunks and
// bytes were written. A send with nothing to write is an error, not a success.
func deliverChunks(blockId string, chunks []string, delay time.Duration, send inputSender, sleep func(time.Duration)) (int, int, error) {
	nChunks, nBytes := 0, 0
	for _, chunk := range chunks {
		if chunk == "" {
			continue
		}
		if nChunks > 0 && delay > 0 {
			sleep(delay)
		}
		if err := send(blockId, []byte(chunk)); err != nil {
			return nChunks, nBytes, fmt.Errorf("sending input to block %s (after %d chunk(s)): %w", blockId, nChunks, err)
		}
		nChunks++
		nBytes += len(chunk)
	}
	if nChunks == 0 {
		return 0, 0, fmt.Errorf("nothing to send: every token was empty")
	}
	return nChunks, nBytes, nil
}

func sendRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("send", rtnErr == nil)
	}()
	tokens := args
	if sendStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		tokens = append(append([]string{}, args...), SendTextPrefix+strings.TrimRight(string(data), "\r\n"))
	}
	if len(tokens) == 0 {
		return fmt.Errorf("nothing to send: give key:<Name> or text tokens, or --stdin")
	}
	chunks, err := encodeSendTokens(tokens, sendRaw)
	if err != nil {
		return err
	}
	fullORef, err := resolveBlockArg()
	if err != nil {
		return err
	}
	metaData, err := wshclient.GetMetaCommand(RpcClient, wshrpc.CommandGetMetaData{ORef: *fullORef}, &wshrpc.RpcOpts{Timeout: 2000})
	if err != nil {
		return fmt.Errorf("block %s: %w", fullORef.OID, err)
	}
	if view, _ := metaData[waveobj.MetaKey_View].(string); view != "term" {
		return fmt.Errorf("block %s is not a terminal block (view type: %s)", fullORef.OID, view)
	}
	send := func(blockId string, data []byte) error {
		return wshclient.ControllerInputCommand(RpcClient, wshrpc.CommandBlockInputData{
			BlockId:     blockId,
			InputData64: base64.StdEncoding.EncodeToString(data),
		}, &wshrpc.RpcOpts{Timeout: 5000})
	}
	nChunks, nBytes, err := deliverChunks(fullORef.OID, chunks, time.Duration(sendDelayMs)*time.Millisecond, send, time.Sleep)
	if err != nil {
		return err
	}
	WriteStdout("wrote %d bytes in %d chunk(s) to the controller of block %s\n", nBytes, nChunks, fullORef.OID)
	return nil
}
