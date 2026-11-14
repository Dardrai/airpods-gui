package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// askUser puts a yes or no question to a person. available is false when nobody can answer,
// so that a script is told what flag it needs instead of hanging. Tests replace it.
var askUser = terminalAsk

func terminalAsk(question string) (yes, available bool) {
	return askOn(os.Stdin, os.Stderr, question)
}

// askOn needs both ends of the conversation: with stderr sent elsewhere, as in 2>/dev/null, the
// question would be invisible and the command would seem to hang.
func askOn(stdin, stderr *os.File, question string) (yes, available bool) {
	if !isTerminal(stdin) || !isTerminal(stderr) {
		return false, false
	}
	return askLine(stdin, stderr, question), true
}

// askLine counts end of input as a no. It ends the question's line, since the error that follows
// would otherwise start on it.
func askLine(stdin io.Reader, stderr io.Writer, question string) bool {
	fmt.Fprint(stderr, question+" [y/N] ")
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(stderr)
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
