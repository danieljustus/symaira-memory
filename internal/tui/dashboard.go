package tui

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/danieljustus/symaira-memory/internal/db"
)

// Styling tokens
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#A2EEEF")).
			Background(lipgloss.Color("#1E1E2E")).
			Padding(1, 2).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("#BAC2DE"))

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#A2EEEF")).
			PaddingLeft(2)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CDD6F4")).
			PaddingLeft(2)

	metaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7F849C")).
			Italic(true)

	statsStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#45475A")).
			Padding(1, 2).
			Width(40)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#585B70")).
			MarginTop(1)

	scopeBadge = func(scope string) string {
		color := "#89B4FA"
		switch scope {
		case "global":
			color = "#A6E3A1"
		case "project":
			color = "#F9E2AF"
		case "agent":
			color = "#CBA6F7"
		case "user":
			color = "#F38BA8"
		case "session":
			color = "#94E2D5"
		}
		return lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#1E1E2E")).
			Background(lipgloss.Color(color)).
			Padding(0, 1).
			Render(" " + strings.ToUpper(scope) + " ")
	}
)

type model struct {
	db        *db.DB
	memories  []*db.Memory
	selected  int
	scope     string
	search    string
	searching bool
	err       error

	dbPath           string
	ollamaURL        string
	ollamaModel      string
	ollamaReachable  bool
	httpPort         int
}

// InitialModel configures state.
func InitialModel(database *db.DB, dbPath, ollamaURL, ollamaModel string, httpPort int) model {
	ollamaReachable := checkOllamaReachable(ollamaURL)
	m := model{
		db:              database,
		scope:           "",
		dbPath:          dbPath,
		ollamaURL:       ollamaURL,
		ollamaModel:     ollamaModel,
		ollamaReachable: ollamaReachable,
		httpPort:        httpPort,
	}
	m.loadMemories()
	return m
}

func checkOllamaReachable(url string) bool {
	if url == "" {
		return false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	baseURL := strings.TrimSuffix(url, "/api/embeddings")
	resp, err := client.Get(baseURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

func (m *model) loadMemories() {
	mems, err := m.db.ListMemories(m.scope)
	if err != nil {
		m.err = err
		return
	}

	if m.search != "" {
		// Basic keyword search inside current scope
		var filtered []*db.Memory
		q := strings.ToLower(m.search)
		for _, mem := range mems {
			if strings.Contains(strings.ToLower(mem.Content), q) {
				filtered = append(filtered, mem)
			}
		}
		m.memories = filtered
	} else {
		m.memories = mems
	}

	if m.selected >= len(m.memories) {
		m.selected = len(m.memories) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Search mode routing
		if m.searching {
			switch msg.String() {
			case "enter":
				m.searching = false
			case "esc":
				m.search = ""
				m.searching = false
				m.loadMemories()
			case "backspace":
				if len(m.search) > 0 {
					m.search = m.search[:len(m.search)-1]
					m.loadMemories()
				}
			default:
				if len(msg.String()) == 1 {
					m.search += msg.String()
					m.loadMemories()
				}
			}
			return m, nil
		}

		// Standard dashboard keys
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}

		case "down", "j":
			if m.selected < len(m.memories)-1 {
				m.selected++
			}

		case "/":
			m.searching = true

		case "d", "backspace":
			if len(m.memories) > 0 {
				target := m.memories[m.selected]
				_ = m.db.DeleteMemory(target.ID)
				m.loadMemories()
			}

		// Filter scope triggers
		case "g":
			m.scope = "global"
			m.selected = 0
			m.loadMemories()
		case "p":
			m.scope = "project"
			m.selected = 0
			m.loadMemories()
		case "a":
			m.scope = "agent"
			m.selected = 0
			m.loadMemories()
		case "u":
			m.scope = "user"
			m.selected = 0
			m.loadMemories()
		case "s":
			m.scope = "session"
			m.selected = 0
			m.loadMemories()
		case "*", "c":
			m.scope = ""
			m.selected = 0
			m.loadMemories()
		}
	}
	return m, nil
}

func (m model) View() string {
	var s strings.Builder

	// Top banner
	s.WriteString(titleStyle.Render("⚡ SYMAIRA MEMORY (symmemory) — CONSOLE"))
	s.WriteString("\n" + subtitleStyle.Render("  The Semantic Memory Layer for the Human-AI Symbiosis Era") + "\n\n")

	if m.err != nil {
		return fmt.Sprintf("Error loading memory console: %v\n", m.err)
	}

	// Dynamic stats column
	activeFilter := "ALL SCOPES"
	if m.scope != "" {
		activeFilter = strings.ToUpper(m.scope)
	}

	ollamaStatus := "down"
	if m.ollamaReachable {
		ollamaStatus = "up"
	}
	httpStatus := "stdio only"
	if m.httpPort > 0 {
		httpStatus = fmt.Sprintf(":%d", m.httpPort)
	}
	statsText := fmt.Sprintf("Active Filter: %s\nTotal Memories: %d\nDB: %s\nOllama: %s (%s %s)\nHTTP: %s",
		activeFilter, len(m.memories), m.dbPath,
		m.ollamaModel, ollamaStatus, m.ollamaURL,
		httpStatus)
	
	s.WriteString(statsStyle.Render(statsText) + "\n\n")

	// Search Indicator
	if m.searching {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#F9E2AF")).Render("🔍 Search Keyword: "+m.search+"_") + "\n\n")
	} else if m.search != "" {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#A6E3A1")).Render("🔍 Active Search: "+m.search+" (Press '/' to edit, 'esc' to clear)") + "\n\n")
	}

	// Main scrollable memory list
	s.WriteString("Persistent Memory Elements:\n")
	s.WriteString("==========================================\n")

	if len(m.memories) == 0 {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#F38BA8")).PaddingLeft(4).Render("No memories match current scope filters.") + "\n")
	} else {
		for i, mem := range m.memories {
			badge := scopeBadge(mem.Scope)
			
			// Highlight selected memory
			if i == m.selected {
				s.WriteString(selectedStyle.Render(fmt.Sprintf("👉 %s  %s", badge, mem.Content)) + "\n")
				s.WriteString(metaStyle.PaddingLeft(6).Render(fmt.Sprintf("ID: %s | Saved: %s", mem.ID, mem.CreatedAt.Format("2006-01-02 15:04"))) + "\n")
			} else {
				s.WriteString(normalStyle.Render(fmt.Sprintf("   %s  %s", badge, mem.Content)) + "\n")
			}
			s.WriteString("\n")
		}
	}

	// Keyboard Controls Footer
	s.WriteString(footerStyle.Render(
		"Controls: [j/k/↑/↓] Navigate | [d/backspace] Delete | [/] Filter Keyword | [g] Global | [p] Project | [a] Agent | [u] User | [s] Session | [*] All Scopes | [q] Exit",
	) + "\n")

	return s.String()
}

// RunDashboard launches the Bubble Tea console.
func RunDashboard(database *db.DB, dbPath, ollamaURL, ollamaModel string, httpPort int) error {
	p := tea.NewProgram(InitialModel(database, dbPath, ollamaURL, ollamaModel, httpPort), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
