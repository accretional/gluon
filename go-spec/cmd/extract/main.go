// Command extract parses the Go specification HTML and splits it into
// per-section files. Each <h2> becomes a top-level file (e.g. Types.html)
// and each <h3> becomes a dotted sub-file (e.g. Types.Struct_types.html).
// The output is written to the same directory as the input file.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: extract <raw_spec.html> [output_dir]\n")
		os.Exit(1)
	}

	inputPath := os.Args[1]
	outputDir := filepath.Dir(inputPath)
	if len(os.Args) >= 3 {
		outputDir = os.Args[2]
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading input: %v\n", err)
		os.Exit(1)
	}

	sections, err := extractSections(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "extracting sections: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "creating output dir: %v\n", err)
		os.Exit(1)
	}

	for _, sec := range sections {
		path := filepath.Join(outputDir, sec.Filename)
		if err := os.WriteFile(path, []byte(sec.Content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "writing %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d bytes)\n", sec.Filename, len(sec.Content))
	}
	fmt.Printf("\nextracted %d files\n", len(sections))
}

// section represents one extracted spec fragment.
type section struct {
	Filename string
	Content  string
}

// extractSections parses the full HTML document, then walks top-level nodes
// splitting on <h2> and <h3> elements to produce per-section fragments.
//
// The approach: since the Go spec HTML is essentially flat (h2/h3/p/pre
// siblings under <body>), we iterate over the body's children. When we hit
// an h2, we start a new top-level section. When we hit an h3, we start a
// subsection under the current h2. All other nodes are appended to the
// current section's buffer.
func extractSections(data []byte) ([]section, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	// Find <body> — html.Parse always produces <html><head><body>.
	body := findElement(doc, atom.Body)
	if body == nil {
		return nil, fmt.Errorf("no <body> element found")
	}

	var sections []section
	var currentH2 string
	var currentH3 string
	var buf bytes.Buffer
	inSection := false

	flush := func() {
		if !inSection {
			return
		}
		content := strings.TrimSpace(buf.String())
		if content == "" {
			return
		}
		var filename string
		if currentH3 != "" {
			filename = currentH2 + "." + currentH3 + ".html"
		} else {
			filename = currentH2 + ".html"
		}
		sections = append(sections, section{
			Filename: filename,
			Content:  content + "\n",
		})
		buf.Reset()
	}

	for child := body.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			id := getAttr(child, "id")

			switch child.DataAtom {
			case atom.H2:
				if id != "" {
					flush()
					currentH2 = id
					currentH3 = ""
					inSection = true
					buf.Reset()
					renderNode(&buf, child)
					continue
				}
			case atom.H3:
				if id != "" {
					flush()
					currentH3 = id
					renderNode(&buf, child)
					continue
				}
			}
		}

		if inSection {
			renderNode(&buf, child)
		}
	}

	flush()
	return sections, nil
}

// findElement performs a depth-first search for the first element with the
// given atom tag.
func findElement(n *html.Node, tag atom.Atom) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElement(c, tag); found != nil {
			return found
		}
	}
	return nil
}

// getAttr returns the value of the named attribute on n, or "".
func getAttr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// renderNode serializes an HTML node (and all its children) back to HTML text.
func renderNode(buf *bytes.Buffer, n *html.Node) {
	if err := html.Render(buf, n); err != nil {
		fmt.Fprintf(os.Stderr, "warning: render error: %v\n", err)
	}
}
