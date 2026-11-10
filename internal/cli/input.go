package cli

import (
	"bufio"
	"fmt"
	"golang.org/x/term"
	"os"
	"strings"
	"sync"
)

// stdin is read through one reader, so a second secret piped in is the
// second line and not lost to the first one's buffer. The reader is made
// over os.Stdin as it is when first read, never at package init: package
// variables are set before any init runs, and on Windows the desktop
// build's init (console_windows.go) only then swaps in the terminal's
// CONIN$. A reader made earlier keeps the invalid handle, and every
// prompt fails with "read /dev/stdin: The handle is invalid".
var (
	stdinMu   sync.Mutex
	stdinFile *os.File
	stdinBuf  *bufio.Reader
)

func stdin() *bufio.Reader {
	stdinMu.Lock()
	defer stdinMu.Unlock()
	if stdinBuf == nil || stdinFile != os.Stdin {
		stdinFile, stdinBuf = os.Stdin, bufio.NewReader(os.Stdin)
	}
	return stdinBuf
}

// secret reads what (a passphrase, a password) as passphrase does; piped
// in, it is the next line of stdin.
func secret(what, prompt string, confirm bool) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := stdin().ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if err != nil {
				return "", fmt.Errorf("no %s on stdin: %w", what, err)
			}
			return "", fmt.Errorf("the %s is empty", what)
		}
		return line, nil
	}
	read := func(p string) (string, error) {
		fmt.Fprint(os.Stderr, p)
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		return string(b), err
	}
	pass, err := read(prompt)
	if err != nil {
		return "", err
	}
	if pass == "" {
		return "", fmt.Errorf("the %s is empty", what)
	}
	if confirm {
		again, err := read("Again: ")
		if err != nil {
			return "", err
		}
		if again != pass {
			return "", fmt.Errorf("the %ss differ", what)
		}
	}
	return pass, nil
}
