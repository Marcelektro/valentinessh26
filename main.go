package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"
	"math/rand"

	"golang.org/x/crypto/ssh"
)

type Session struct {
	LastSentIndex int
}

func main() {
	host := flag.String("host", "0.0.0.0", "host to bind to")
	port := flag.Int("port", 2717, "port to listen on")
	keyFile := flag.String("key", "ssh_host_key", "path to private host key file (defaults to ssh_host_key)")
	flag.Parse()

	var hostKeyBytes []byte
	hostKeyBytes, err := os.ReadFile(*keyFile)
	if err != nil {
		log.Printf("Warning: failed to read key file %s: %v", *keyFile, err)
	    os.Exit(-1)
	}

	hostKey, err := ssh.ParsePrivateKey(hostKeyBytes)
	if err != nil {
		log.Fatalf("Failed to parse host key: %v", err)
	}

	config := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	config.AddHostKey(hostKey)

	listenAddr := fmt.Sprintf("%s:%d", *host, *port)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", listenAddr, err)
	}
	defer listener.Close()

	log.Printf("SSH server running on %s. Connect with: ssh <server> -p %d", listenAddr, *port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		go handleConnection(conn, config)
	}
}

func handleConnection(conn net.Conn, config *ssh.ServerConfig) {
	defer conn.Close()

	sshConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		log.Printf("SSH handshake failed: %v", err)
		return
	}
	defer sshConn.Close()

	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Printf("Could not accept channel: %v", err)
			continue
		}

		// new session
		session := &Session{LastSentIndex: -1}

		go handleChannel(channel, requests, session)
	}
}

func handleChannel(channel ssh.Channel, requests <-chan *ssh.Request, session *Session) {
	defer channel.Close()

	for req := range requests {
		if req.Type == "pty-req" {
			// accept pty so the client allocates a terminal
			req.Reply(true, []byte{})
			continue
		}

		if req.Type == "shell" {
			req.Reply(true, []byte{})
			interactiveSession(channel, session)
			return
		}

		req.Reply(false, []byte{})
	}
}

func clearScreen(channel ssh.Channel) {
	fmt.Fprint(channel, "\033[2J\033[H")
}

func clearLine(channel ssh.Channel) {
	fmt.Fprint(channel, "\033[2K\r")
}

func clearLinesRel(channel ssh.Channel, n int) {
	
	for i := 0; i < n; i++ {
		clearLine(channel)
		if i < n-1 {
			moveCursorRel(channel, 0, 1)
		}
	}
	moveCursorRel(channel, 0, -(n-1))
}

func printMulti(channel ssh.Channel, text string) {
	fmt.Fprint(channel, strings.ReplaceAll(text, "\n", "\r\n"))
}

func printNewLines(channel ssh.Channel, n int) {
	fmt.Fprint(channel, strings.Repeat("\r\n", n))
}

/**
 * Moves the cursor relative to its current position.
 * Positive x moves right, negative x moves left.
 * Positive y moves down, negative y moves up.
 */
func moveCursorRel(channel ssh.Channel, x, y int) {
	if y > 0 {
		fmt.Fprintf(channel, "\033[%dB", y) // Move down
	} else if y < 0 {
		fmt.Fprintf(channel, "\033[%dA", -y) // Move up
	}

	if x > 0 {
		fmt.Fprintf(channel, "\033[%dC", x) // Move right
	} else if x < 0 {
		fmt.Fprintf(channel, "\033[%dD", -x) // Move left
	}
}

func moveCursorAbs(channel ssh.Channel, x, y int) {
	// ANSI escape codes are 1-based for row and column
	fmt.Fprintf(channel, "\033[%d;%dH", y+1, x+1)
}


func interactiveSession(channel ssh.Channel, session *Session) {

    clearScreen(channel)
	printNewLines(channel, 2)

	printMulti(channel, strings.ReplaceAll(heartASCII, "💙", "  "))

	printNewLines(channel, 2)
	
	// printMulti(channel, promptBox)
	// moveCursorRel(channel, promptBoxPromptRelativeOffsetX, promptBoxPromptRelativeOffsetY)

	
	for {

		moveCursorAbs(channel, 0, 10)

		printMulti(channel, promptBox)
		moveCursorRel(channel, promptBoxPromptRelativeOffsetX, promptBoxPromptRelativeOffsetY)


		input, err := readLineWithEcho(channel)
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading input: %v", err)
			}
			return
		}

		moveCursorAbs(channel, 0, 17)

		input = strings.TrimSpace(input)

		if input == "" {
			clearLinesRel(channel, 7)
			fmt.Fprint(channel, "Please enter something!\r\n")
			continue
		}

		if input == "exit" || input == "quit" {
			break
			goto end
		}


		if input != "<33" {
			clearLinesRel(channel, 2)

			rand.Seed(time.Now().UnixNano())

			texts := []string{
				"That's a nice one, but try something else! I'm sure you can figure it out!",
				"Interesting input! It's not the right one though.",
				"Hmm, not quite right! Pay close attention to every detail today.",
				"The key is hidden in plain sight! Look closely at something you got from someone you love!",
			}

			idx := rand.Intn(len(texts))
			if len(texts) > 1 && idx == session.LastSentIndex {
				idx = (idx + 1) % len(texts)
			}
			
			fmt.Fprintf(channel, "%s\r\n", texts[idx])
			session.LastSentIndex = idx

			fmt.Fprint(channel, "To give up, use \"quit\".\r\n")
			continue
		}


		clearScreen(channel)
		printNewLines(channel, 1)

		typewriterEffect(channel, "You found the secret key! ", 50*time.Millisecond)
		time.Sleep(500 * time.Millisecond)
		typewriterEffect(channel, "Well done!!!\r\n", 40*time.Millisecond)
		time.Sleep(1000 * time.Millisecond)
		typewriterEffect(channel, "Initiating Valentine's Day surprise...\r\n", 20*time.Millisecond)
		time.Sleep(1000 * time.Millisecond)

		animateHearts(channel, 0, 4)

		printTypedLine := func(text string, delayMs int, extraPauseAfter int) {
			typewriterEffect(channel, "  " + text + "\r\n", time.Duration(delayMs)*time.Millisecond)
			if extraPauseAfter > 0 {
				time.Sleep(time.Duration(extraPauseAfter) * time.Millisecond)
			}
		}


		// letterahhhh
		printNewLines(channel, 2)
		printTypedLine("                    *** Happy Valentine's Day!!! ***", 50, 1000)
		printTypedLine("Hello my dearest Molly!", 30, 100)
		printNewLines(channel, 1)
		printTypedLine("Today is a beautiful day to express my deepest feelings for you.", 30, 300)
		printTypedLine("You are the light of my life, the beat of my heart, and the love of my soul.", 30, 400)
		printTypedLine("I am so grateful to have you in my life, and I promise I will always be there for you.", 30, 500)
		printNewLines(channel, 1)
		printTypedLine("No words can possibly capture what I feel for you, but I hope this little", 30, 0)
		printTypedLine(" silly SSH surprise brings a nerdy smile to your face.", 30, 1000)
		printNewLines(channel, 1)
		printTypedLine("I love you sososososo much and I would like to", 30, 0)
		printTypedLine(" wish you the HAPPIEST VALENTINE'S DAY EVER!!! <333", 30, 1000)
		printNewLines(channel, 1)
		printTypedLine("~ Yours forever,", 30, 150)
		typewriterEffect(channel, "  marcelektro@bf4l.net <3", 30*time.Millisecond)

		break
		goto end
		
	}

	end: 
	fmt.Fprint(channel, "\r\n\r\nSee you soon!\r\n")

}


func typewriterEffect(channel ssh.Channel, text string, delay time.Duration) {
	for idx, r := range text {

		// do underline only if char is visible ascii
		doUnderline := r >= 32 && r <= 126 && idx < len(text)-1
		doSleep := delay > 0 && idx < len(text)-1 && r != '\r' && r != '\n' && r != ' '

		fmt.Fprint(channel, string(r))
		if doUnderline {
			fmt.Fprint(channel, "_")
		}
		if doSleep {
			time.Sleep(delay + time.Duration(rand.Intn(100))*time.Millisecond)
		}
		if doUnderline {
			fmt.Fprint(channel, "\b \b") // erase the underscore
		}
	}
}


// copilot-generated, kinda buggy xD
// will rewrite myself later (im short on time)
func animateHearts(channel ssh.Channel, offsetX, offsetY int) {
	// Diagonal wave animation that replaces pink hearts (🩷) with blue (💙)
	// Put cursor absolutely to top-left before drawing each frame
	lines := strings.Split(strings.Trim(heartASCIITransparent, "\n"), "\n")
	h := len(lines)
	w := 0
	for _, ln := range lines {
		if lw := len([]rune(ln)); lw > w {
			w = lw
		}
	}

	// Build rune grid for safe emoji handling
	grid := make([][]rune, h)
	for i := 0; i < h; i++ {
		r := []rune(lines[i])
		if len(r) < w {
			padded := make([]rune, w)
			copy(padded, r)
			for k := len(r); k < w; k++ {
				padded[k] = ' '
			}
			r = padded
		}
		grid[i] = r
	}

	maxStep := h + w

	// Run a few cycles of the diagonal covering (top-left -> bottom-right)
	cycles := 1
	delay := 80 * time.Millisecond
	for c := 0; c < cycles; c++ {
		for step := 0; step <= maxStep; step++ {
			moveCursorAbs(channel, offsetX, offsetY)
			var b strings.Builder
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					orig := grid[y][x]
					// Only replace pink hearts (🩷) with blue (💙) when covered by diagonal
					if y+x <= step && orig == '🩷' {
						b.WriteString("💙")
					} else {
						b.WriteString(string(orig))
					}
				}
				if y < h-1 {
					b.WriteString("\r\n")
				}
			}
			fmt.Fprint(channel, b.String())
			time.Sleep(delay)
		}
	}

	// Ensure fully replaced state (all pink -> blue)
	moveCursorAbs(channel, offsetX, offsetY)
	var finalBuilder strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if grid[y][x] == '🩷' {
				finalBuilder.WriteString("💙")
			} else {
				finalBuilder.WriteString(string(grid[y][x]))
			}
		}
		if y < h-1 {
			finalBuilder.WriteString("\r\n")
		}
	}
	fmt.Fprint(channel, finalBuilder.String())

	// Fade-out diagonally the previously-pink (now-blue) hearts in the same order
	time.Sleep(300 * time.Millisecond)
	for step := 0; step <= maxStep; step++ {
		moveCursorAbs(channel, offsetX, offsetY)
		var b strings.Builder
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				orig := grid[y][x]
				// If this cell was originally pink and is within the fade step, show space
				if y+x <= step && orig == '🩷' {
					b.WriteString("🩷")
				} else if orig == '🩷' {
					// still show blue for pink-origin cells not yet faded
					b.WriteString("💙")
				} else {
					b.WriteString(string(orig))
				}
			}
			if y < h-1 {
				b.WriteString("\r\n")
			}
		}
		fmt.Fprint(channel, b.String())
		time.Sleep(delay)
	}

}

func readLineWithEcho(rw io.ReadWriter) (string, error) {
	reader := bufio.NewReader(rw)
	var buf []rune

	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			return "", err
		}

		// CR, LF, or CRLF as end-of-line
		if r == '\r' || r == '\n' {
			break
		}

		// Backspace (127)
		if r == 127 {
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				// erase last char on client
				_, _ = rw.Write([]byte("\b \b"))
			}
			continue
		}

		// Handle ctrl+c or ctrl+d or ctrl+x to exit input
		if r == 3 || r == 4 || r == 24 {
			// _, _ = rw.Write([]byte("\033[2J\033[H")) // clear screen
			return "", io.EOF
		}

		// only append visible characters (printable ASCII and UTF-8)
		if r >= 32 && r != 127 {
			buf = append(buf, r)
			_, _ = rw.Write([]byte(string(r)))
		}
		continue
		_, _ = rw.Write([]byte(string(r)))
	}

	return string(buf), nil
}
