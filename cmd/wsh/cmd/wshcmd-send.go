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

	"github.com/spf13/cobra"
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
to put on a command line).`,
	Example: `  wsh send -b 3 key:Down key:Enter
  wsh send -b 3f2a9c10 "text:continue please" key:Enter
  wsh send -b 3 key:Ctrl+C`,
	RunE:    sendRun,
	PreRunE: preRunSetupRpcClient,
}

var (
	sendDelayMs int
	sendStdin   bool
)

func init() {
	sendCmd.Flags().IntVar(&sendDelayMs, "delay", 40, "pause in milliseconds between tokens")
	sendCmd.Flags().BoolVar(&sendStdin, "stdin", false, "also send standard input as literal text, last")
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

// encodeSendTokens turns tokens into the chunks that are written one at a time.
func encodeSendTokens(tokens []string) ([]string, error) {
	chunks := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		switch {
		case strings.HasPrefix(tok, SendKeyPrefix):
			val, err := encodeKey(tok[len(SendKeyPrefix):])
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, val)
		case strings.HasPrefix(tok, SendTextPrefix):
			chunks = append(chunks, tok[len(SendTextPrefix):])
		default:
			chunks = append(chunks, tok)
		}
	}
	return chunks, nil
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
		tokens = append(append([]string{}, args...), SendTextPrefix+string(data))
	}
	if len(tokens) == 0 {
		return fmt.Errorf("nothing to send: give key:<Name> or text tokens, or --stdin")
	}
	chunks, err := encodeSendTokens(tokens)
	if err != nil {
		return err
	}
	fullORef, err := resolveBlockArg()
	if err != nil {
		return err
	}
	for i, chunk := range chunks {
		if i > 0 && sendDelayMs > 0 {
			time.Sleep(time.Duration(sendDelayMs) * time.Millisecond)
		}
		if chunk == "" {
			continue
		}
		data := wshrpc.CommandBlockInputData{
			BlockId:     fullORef.OID,
			InputData64: base64.StdEncoding.EncodeToString([]byte(chunk)),
		}
		err = wshclient.ControllerInputCommand(RpcClient, data, &wshrpc.RpcOpts{Timeout: 5000})
		if err != nil {
			return fmt.Errorf("sending input to block %s: %w", fullORef.OID, err)
		}
	}
	WriteStdout("sent %d token(s) to block %s\n", len(chunks), fullORef.OID)
	return nil
}
