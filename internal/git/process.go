package git

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// process keeps one git command running for a whole run: starting git costs
// 50-80 ms on Windows, so a process per file made a 1000-file commit take a
// minute.
type process struct {
	name   string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr bytes.Buffer
}

func startProcess(dir string, args ...string) (*process, error) {
	cmd := exec.Command("git", args...) //nolint:gosec // git subcommand with repo-relative paths the user asked to format
	cmd.Dir = dir

	p := &process{name: args[0], cmd: cmd}
	cmd.Stderr = &p.stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", p.name, err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", p.name, err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start git %s: %w", p.name, err)
	}

	p.stdin = stdin
	p.stdout = bufio.NewReader(stdout)

	return p, nil
}

func (p *process) send(line string) error {
	if _, err := io.WriteString(p.stdin, line+"\n"); err != nil {
		return p.failed(err)
	}

	return nil
}

func (p *process) readLine() (string, error) {
	line, err := p.stdout.ReadString('\n')
	if err != nil {
		return "", p.failed(err)
	}

	return strings.TrimSuffix(line, "\n"), nil
}

// failed adds what git printed before it died, which says far more than the
// broken pipe or EOF the caller saw.
func (p *process) failed(err error) error {
	if msg := strings.TrimSpace(p.stderr.String()); msg != "" {
		return fmt.Errorf("git %s: %s: %w", p.name, msg, err)
	}

	return fmt.Errorf("git %s: %w", p.name, err)
}

func (p *process) request(line string) (string, error) {
	if err := p.send(line); err != nil {
		return "", err
	}

	return p.readLine()
}

// close waits for git to exit even when closing stdin fails, so no process is
// left behind.
func (p *process) close() error {
	closeErr := p.stdin.Close()

	if err := errors.Join(closeErr, p.cmd.Wait()); err != nil {
		return p.failed(err)
	}

	return nil
}

// readBatchBlob reads one `git cat-file --batch` reply: a
// "<oid> <type> <size>" header, the content, then a newline. A name git cannot
// resolve gets a "<name> missing" header and no content.
func readBatchBlob(r *bufio.Reader) ([]byte, error) {
	header, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read reply header: %w", err)
	}

	fields := strings.Fields(header)
	if len(fields) != 3 {
		return nil, fmt.Errorf("git replied %q", strings.TrimSpace(header))
	}

	if fields[1] != "blob" {
		return nil, fmt.Errorf("a %s, not a blob", fields[1])
	}

	size, err := strconv.Atoi(fields[2])
	if err != nil {
		return nil, fmt.Errorf("bad size %q: %w", fields[2], err)
	}

	content := make([]byte, size+1)
	if _, err := io.ReadFull(r, content); err != nil {
		return nil, fmt.Errorf("read content: %w", err)
	}

	if content[size] != '\n' {
		return nil, errors.New("cat-file reply out of step: no newline after the content")
	}

	return content[:size], nil
}

// quotePath writes path the way git's --stdin-paths unquotes a line that starts
// with a double quote. Sent bare, a path with a newline would split into two
// requests and put every later reply out of step, and git would strip a
// trailing CR.
func quotePath(path string) string {
	var b strings.Builder

	b.WriteByte('"')

	for i := range len(path) {
		c := path[i]

		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c < ' ' || c == 0x7f:
			fmt.Fprintf(&b, "\\%03o", c)
		default:
			b.WriteByte(c)
		}
	}

	b.WriteByte('"')

	return b.String()
}
