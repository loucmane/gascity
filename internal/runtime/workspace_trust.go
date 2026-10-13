package runtime

import (
	"fmt"
	"strings"
)

// Strip only terminal decoration and scrollback. The complete two-option
// Claude menu must still validate before a move or confirmation is sent.
func normalizedWorkspaceTrustMenu(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = stripLeadingBoxBorder(strings.TrimSpace(line))
	}
	content = strings.Join(lines, "\n")
	if start := strings.LastIndex(content, "Accessing workspace:"); start >= 0 {
		content = content[start:]
	}
	return content
}

func validateWorkspaceTrustMenu(content string) error {
	if strings.Contains(content, "Quick safety check") || strings.Contains(content, "trust this folder") {
		_, err := workspaceTrustKeys(normalizedWorkspaceTrustMenu(content))
		return err
	}
	return nil
}

// workspaceTrustKeys recognizes the complete Claude menu before sending any
// input. Selection order is provider UI state, not an authorization signal:
// newer Claude clients default to "No, exit". Other providers retain their
// existing startup contracts.
func workspaceTrustKeys(content string) ([]string, error) {
	if !strings.Contains(content, "Quick safety check") && !strings.Contains(content, "trust this folder") {
		return []string{"Enter"}, nil
	}
	refuse := func() ([]string, error) {
		return nil, fmt.Errorf("unrecognized Claude workspace trust menu; no input sent")
	}
	const footer = "Enter to confirm · Esc to cancel"
	if !strings.Contains(content, "Accessing workspace:") || !strings.Contains(content, "Quick safety check") || strings.Count(content, footer) != 1 {
		return refuse()
	}
	before, _, _ := strings.Cut(content, footer)
	lines := strings.Split(strings.TrimSpace(before), "\n")
	// The menu is the final nonempty block before the confirmation footer.
	// Requiring exactly two rows rejects additional/unknown choices instead of
	// guessing how far a cursor movement would travel.
	start := len(lines)
	for start > 0 && strings.TrimSpace(lines[start-1]) != "" {
		start--
	}
	if len(lines)-start != 2 || strings.Count(before, "❯") != 1 {
		return refuse()
	}
	selected, yes, no := -1, -1, -1
	numbered := false
	for i, raw := range lines[start:] {
		row := strings.TrimSpace(raw)
		if strings.HasPrefix(row, "❯") {
			selected = i
			row = strings.TrimSpace(strings.TrimPrefix(row, "❯"))
		}
		prefix := fmt.Sprintf("%d. ", i+1)
		isNumbered := strings.HasPrefix(row, prefix)
		if i == 0 {
			numbered = isNumbered
		} else if numbered != isNumbered {
			return refuse()
		}
		row = strings.TrimPrefix(row, prefix)
		switch row {
		case "Yes, I trust this folder":
			yes = i
		case "No, exit":
			no = i
		default:
			return refuse()
		}
	}
	if selected < 0 || yes < 0 || no < 0 {
		return refuse()
	}
	if selected == yes {
		return []string{"Enter"}, nil
	}
	if selected < yes {
		return []string{"Down", "Enter"}, nil
	}
	return []string{"Up", "Enter"}, nil
}
