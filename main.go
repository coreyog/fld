package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/jessevdk/go-flags"
	"github.com/mattn/go-runewidth"
	"github.com/nsf/termbox-go"
)

//go:embed VERSION
var version string

type Positional struct {
	TargetFile string `positional-arg-name:"FILE" description:"Target file to be processed"`
}

type Arguments struct {
	ShowVersion bool   `short:"v" long:"version" description:"Show version information"`
	Formatter   string `short:"f" long:"format" description:"What formatter to use"`
	Positional  `positional-args:"yes"`
}

type State string

const (
	StateViewing   State = "viewing"
	StateSearching State = "searching"
)

var args *Arguments = &Arguments{}
var sizeX, sizeY int // size of the terminal
var cursor int       // cursor position, what line relative to the terminal is the cursor on (ignores viewY offset)
var viewX, viewY int // the offset of the view, what line is the top left of the terminal
var smIndent int
var state State = StateViewing
var searchTerm string
var lines []*Line
var format string
var cursorLineNum = 0 // the logical line number the cursor is on
var notFound bool
var debug bool

func main() {
	_, err := flags.Parse(args)
	if err != nil && !flags.WroteHelp(err) {
		panic(err)
	}

	if args.ShowVersion || flags.WroteHelp(err) {
		fmt.Println(version)
		return
	}

	// intake
	if args.Formatter != "" {
		lines, format, smIndent, err = ReadAndFormat(args.TargetFile, args.Formatter)
	} else {
		lines, format, smIndent, err = ReadAndFormat(args.TargetFile)
	}

	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1) // haven't init'd termbox yet, so we can use this here
	}

	// prepare the terminal
	err = termbox.Init()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer termbox.Close()

	// main loop
	exitting := false
	for !exitting {
		// draw
		sizeX, sizeY = termbox.Size()
		err = termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
		if err != nil {
			fmt.Println(err)
			exitting = true
			continue
		}

		// rendering intermediates
		out := bytes.NewBuffer(make([]byte, 0, sizeX))
		index := viewY    // the index of the line we're rendering
		lastLine := -1    // last line of output
		longestLine := -1 // total length of the longest line printed (not truncated by sizeX)

		for i := 0; i < sizeY-2; i++ {
			if index >= len(lines) {
				break
			}

			l := lines[index]
			index++
			out.Reset()

			if l.Hidden {
				i--
				continue
			}

			lastLine = i

			if i == cursor {
				cursorLineNum = l.Index
			}

			if l.CanFold {
				if l.IsFolded {
					out.WriteRune('+')
				} else {
					out.WriteRune('-')
				}
			} else {
				out.WriteRune(' ')
			}

			out.WriteRune('│')
			available := sizeX - out.Len()
			if len(l.Content)-viewX > available {
				out.WriteString(l.Content[viewX : viewX+available-3])
				out.WriteString("...")
			} else if len(l.Content) > viewX {
				out.WriteString(l.Content[viewX:])
			}

			if len(l.Content) > longestLine {
				longestLine = len(l.Content)
			}

			tbprint(0, i, termbox.ColorDefault, termbox.ColorDefault, out.String())
		}

		tbprint(0, sizeY-2, termbox.ColorDefault, termbox.ColorDefault, fmt.Sprintf("─┴%s", strings.Repeat("─", sizeX-2)))

		statusBarLeft := fmt.Sprintf("Line: %s", humanize.Commaf(float64(cursorLineNum+1)))
		statusBarRight := fmt.Sprintf("Format: %s", format)

		tbprint(sizeX-len(statusBarRight), sizeY-1, termbox.ColorDefault, termbox.ColorDefault, statusBarRight)

		// if searching, show search in status bar
		if state == StateSearching {
			termOut := searchTerm
			availableSearchSpace := sizeX - 48 // 20 for the line number + 8 for the "Search: " label + 20 for the Format = 40

			if len(termOut) > availableSearchSpace {
				termOut = "..." + termOut[len(termOut)-availableSearchSpace:]
			}

			statusBarSearch := fmt.Sprintf("Search: %s▏", termOut)
			tbprint(20, sizeY-1, termbox.ColorDefault, termbox.ColorDefault, statusBarSearch)
		} else if notFound {
			// did a search come up empty? Not found
			tbprint(20, sizeY-1, termbox.ColorDefault, termbox.ColorDefault, "Not found")
		}

		tbprint(0, sizeY-1, termbox.ColorDefault, termbox.ColorDefault, statusBarLeft)

		if lastLine < cursor {
			// cursor is passed the end of the doc, probably as a result of a big fold
			viewX = 0
			viewY = 0
			cursor = 0

			continue
		}

		if longestLine-viewX+2 < sizeX && viewX > 0 {
			// keep the view from being too far to the right
			viewX = longestLine - sizeX + 2
			if viewX < 0 {
				viewX = 0
			}

			continue
		}

		if debug {
			msg := fmt.Sprintf("Size: (%d, %d)", sizeX, sizeY)
			tbprint(sizeX-len(msg), 1, termbox.ColorWhite, termbox.ColorBlack, msg)

			msg = fmt.Sprintf("View: (%d, %d)", viewX, viewY)
			tbprint(sizeX-len(msg), 2, termbox.ColorWhite, termbox.ColorBlack, msg)

			msg = fmt.Sprintf("Cursor: %d, (%d)", cursor, cursorLineNum)
			tbprint(sizeX-len(msg), 3, termbox.ColorWhite, termbox.ColorBlack, msg)
		}

		c := termbox.GetCell(0, cursor)
		termbox.SetCell(0, cursor, c.Ch, termbox.ColorDefault|termbox.AttrReverse, termbox.ColorDefault|termbox.AttrReverse)

		// draw buffer
		termbox.Flush()

		// input
		evt := termbox.PollEvent()

		// handle input
		switch state {
		case StateSearching:
			if evt.Ch != 0 {
				searchTerm += string(evt.Ch)
			} else {
				switch evt.Key {
				case termbox.KeyEsc:
					state = StateViewing
				case termbox.KeyBackspace, termbox.KeyBackspace2:
					if len(searchTerm) > 0 {
						searchTerm = searchTerm[:len(searchTerm)-1]
					} else {
						state = StateViewing
					}
				case termbox.KeyEnter:
					state = StateViewing
					findNext()
				}

				if evt.Key != termbox.KeyEnter {
					notFound = false
				}
			}
		case StateViewing:
			if evt.Ch == 0 {
				switch evt.Key {
				case termbox.KeyEsc, termbox.KeyCtrlC:
					exitting = true
				case termbox.KeyArrowUp:
					cursor--
					if cursor < 0 {
						cursor = 0
						viewY--

						for viewY >= 0 && lines[viewY].Hidden {
							viewY--
						}

						if viewY < 0 {
							viewY = 0
						}
					}
				case termbox.KeyArrowDown:
					cursor++
					if cursor >= sizeY-2 {
						cursor = sizeY - 2
						viewY++

						for viewY < len(lines) && lines[viewY].Hidden {
							viewY++
						}

						if viewY >= len(lines) {
							viewY = len(lines) - sizeY
						}
					}
					if cursor > lastLine {
						cursor = lastLine
					}
				case termbox.KeyArrowLeft:
					viewX--
					if viewX < 0 {
						viewX = 0
					}
				case termbox.KeyArrowRight:
					viewX++
				case termbox.KeySpace:
					fold(cursorLineNum)
				case termbox.KeyCtrlF:
					state = StateSearching

					if notFound {
						searchTerm = ""
						notFound = false
					}
				case termbox.KeyCtrlN:
					findNext()
				}

				if evt.Key != termbox.KeyCtrlF && evt.Key != termbox.KeyCtrlN {
					notFound = false
				}
			} else {
				switch evt.Ch {
				case 'q': // quit
					exitting = true
				case 'f': // fold
					setAll(lines, true)
				case 'u': // unfold
					setAll(lines, false)
				case 'd': // debug
					debug = !debug
				}

				notFound = false
			}
		}
	}
}

func fold(index int) {
	// find the target line be working backwards
	var target *Line
	for i := index; i >= 0; i-- {
		if !lines[i].Hidden && lines[i].CanFold {
			target = lines[i]
			break
		}
	}

	// no target line? nothing to fold
	if target == nil {
		return
	}

	// toggle that line
	target.IsFolded = !target.IsFolded

	// mark all children lines as hidden
	subFoldDepth := -1
	for i := target.Index + 1; i < len(lines); i++ {
		if subFoldDepth != -1 {
			if lines[i].Indention > subFoldDepth {
				continue
			} else {
				subFoldDepth = -1
			}
		}

		if lines[i].Indention > target.Indention {
			if lines[i].IsFolded {
				subFoldDepth = lines[i].Indention
			}

			lines[i].Hidden = target.IsFolded
		} else {
			break
		}
	}
}

func setAll(lines []*Line, f bool) {
	for _, l := range lines {
		if l.CanFold {
			l.IsFolded = f
		}

		if l.Indention > smIndent {
			l.Hidden = f
		}
	}
}

func findNext() {
	start := cursorLineNum + 1
	if notFound {
		start = 0
	}

	for i := start; i < len(lines); i++ {
		if strings.Contains(strings.ToLower(lines[i].Content), strings.ToLower(searchTerm)) {
			viewY = i
			cursor = 0

			for lines[i].Hidden {
				for backUp := i; backUp >= 0; backUp-- {
					if lines[backUp].CanFold && lines[backUp].IsFolded && !lines[backUp].Hidden {
						fold(backUp)
					}
				}
			}

			notFound = false

			return
		}
	}

	notFound = true
}

func tbprint(x, y int, fg, bg termbox.Attribute, msg string) {
	for _, c := range msg {
		termbox.SetCell(x, y, c, fg, bg)
		x += runewidth.RuneWidth(c)
		if c == '\t' {
			x += Cfg.TabSize
		}
	}
}
