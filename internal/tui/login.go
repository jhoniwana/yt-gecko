package tui

import (
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/jhoniwana/yt-gecko/internal/auth"
)

const youtubeSignInURL = "https://www.youtube.com/account"

type loginResultMsg struct {
	browser auth.Browser
	err     error
}

type recheckMsg struct {
	browser auth.Browser
	err     error
}

func (m *Model) renderLogin() string {
	var b strings.Builder

	b.WriteString(m.styles.Accent.Render("Login with your browser") + "\n\n")
	b.WriteString("yt-gecko reuses the cookies from a browser you have already\n")
	b.WriteString("signed into YouTube with. Choose a browser, press Enter to\n")
	b.WriteString("verify its cookies, or press o to sign in to YouTube in your\n")
	b.WriteString("browser now.\n\n")

	if len(m.browsers) == 0 {
		b.WriteString(m.styles.Warn.Render("No supported browser profiles were found on this system.") + "\n")
		b.WriteString(m.styles.Hint.Render("Install or run a browser once and sign in to YouTube in it.") + "\n")
		return b.String()
	}

	for i, br := range m.browsers {
		if i == m.loginCur {
			b.WriteString(m.styles.Active.Render("> " + br.Label()))
		} else {
			b.WriteString("  " + br.Label())
		}
		b.WriteString("\n")
	}

	if m.busy {
		b.WriteString("\n" + m.styles.Hint.Render("Checking "+m.browsers[m.loginCur].Label()+" cookies...") + "\n")
	}

	b.WriteString("\n" + m.styles.Help.Render("[o] open browser & sign in    [Enter] verify    [Esc] back") + "\n")
	return m.styles.Box.Render(b.String())
}

func (m *Model) updateLogin(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "o", "O":
		return openBrowser(youtubeSignInURL)
	case "j", "down":
		m.moveLogin(1)
	case "k", "up":
		m.moveLogin(-1)
	case "enter":
		if !m.busy && len(m.browsers) > 0 {
			return m.loginWith(m.browsers[m.loginCur])
		}
	case "esc", "q", "Q":
		m.mode = modeHome
	}
	return nil
}

func (m *Model) moveLogin(delta int) {
	if len(m.browsers) == 0 {
		return
	}
	m.loginCur += delta
	if m.loginCur < 0 {
		m.loginCur = 0
	}
	if m.loginCur >= len(m.browsers) {
		m.loginCur = len(m.browsers) - 1
	}
}

func (m *Model) loginWith(b auth.Browser) tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		return loginResultMsg{browser: b, err: auth.VerifyBrowserCookies(b)}
	}
}

// recheckCmd verifies the saved browser's cookies in the background on
// startup, without opening the browser or touching the current feed.
func (m *Model) recheckCmd() tea.Cmd {
	b := m.core.Browser()
	if b == "" {
		return nil
	}
	return func() tea.Msg {
		return recheckMsg{browser: b, err: auth.VerifyBrowserCookies(b)}
	}
}

// openBrowser opens a URL in the user's default browser. It returns an
// error message instead of a failing tea command.
func openBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		if err := exec.Command("xdg-open", url).Start(); err != nil {
			return errMsg{err: err}
		}
		return nil
	}
}
