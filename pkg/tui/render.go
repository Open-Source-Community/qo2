package tui

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ////////////////////////////////////////////////////////////////////////////////////////////////////////
const (
	logFile   = "debug.log"
	stateFile = "state.json"
)

type PersistedState struct {
	Version         int              `json:"version"`
	Registration    registrationInfo `json:"registration"`
	Answers         []string         `json:"answers"`
	CurrentQuestion int              `json:"current_question"`
	Completed       bool             `json:"completed"`
	LastUpdated     time.Time        `json:"last_updated"`
}

func saveState(m model) {
	state := PersistedState{
		Version:         1,
		Registration:    m.registration,
		CurrentQuestion: m.currentQuestion,
		Completed:       m.currentQuestion == -1,
		LastUpdated:     time.Now(),
	}

	state.Answers = make([]string, 0, len(m.questionSet.questions))
	for _, q := range m.questionSet.questions {
		state.Answers = append(state.Answers, q.answer)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		log.Printf("state save failed: %v", err)
		return
	}

	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		log.Printf("state write failed: %v", err)
	}
}

func loadState(m model) model {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		log.Printf("no previous state found")
		return m
	}

	var state PersistedState
	if err := json.Unmarshal(data, &state); err != nil {
		log.Printf("state parse error: %v", err)
		return m
	}

	m.registration = state.Registration

	fields := []string{
		state.Registration.name,
		state.Registration.level,
		state.Registration.id,
		state.Registration.email,
	}

	for i := range m.registrationFields {
		m.registrationFields[i].SetValue(fields[i])
	}
	m.currentQuestion = state.CurrentQuestion
	m.screen = questionScreen

	for i := range m.questionSet.questions {
		if i < len(state.Answers) {
			m.questionSet.questions[i].answer = state.Answers[i]
			m.questionSet.questions[i].input.SetValue(state.Answers[i])
		}
	}

	answered := state.CurrentQuestion
	if answered < 0 {
		answered = len(m.questionSet.questions)
	}

	m.progress.SetPercent(
		float64(answered) / float64(len(m.questionSet.questions)),
	)

	m.restoreProgress = true

	log.Printf("state restored: question %d", m.currentQuestion)
	return m
}

// resume session helper
func resumeSession(m model) model {
	m.currentQuestion = 0
	m.registration = registrationInfo{}
	m.registrationIdx = 0
	m.progress.SetPercent(0)

	for i := range m.registrationFields {
		m.registrationFields[i].SetValue("")
	}
	for i := range m.questionSet.questions {
		m.questionSet.questions[i].answer = ""
		m.questionSet.questions[i].input.SetValue("")
	}

	_ = os.Remove(stateFile)
	return m
}

func (m model) restoreView() string {
	options := []string{
		"Resume previous session",
		"Start a new session",
	}

	var lines []string
	for i, opt := range options {
		cursor := "  "
		if i == m.restoreChoice {
			cursor = "> "
		}
		lines = append(lines, cursor+opt)
	}

	help := "• ↑/↓ to choose • Enter to confirm • ctrl+c to quit"

	return containerStyle.Render(
		lipgloss.JoinVertical(lipgloss.Top,
			boldStyle.Render("Previous session found"),
			"\n",
			subtleStyle.Render("What would you like to do?"),
			"\n",
			lipgloss.JoinVertical(lipgloss.Left, lines...),
			"\n",
			subtleStyle.Render(help),
		),
	)
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////
// styling and formatting
const (
	maxWidth            = 50
	hexBlue      string = "68b0f4"
	hexLightBlue string = "bddfff"
)

var (
	containerStyle = lipgloss.NewStyle().
			Margin(2)
	boldStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#" + hexBlue)).
			Bold(true)
	subtleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#" + hexLightBlue))
	borderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#" + hexBlue)).
			Border(lipgloss.RoundedBorder())
	questionStyle = lipgloss.NewStyle().Width(maxWidth)
)

// my screens
type screen int

const (
	restoreScreen screen = iota
	registrationScreen
	questionScreen
)

type Input interface {
	Value() string
	Blur() tea.Msg
	Update(tea.Msg) (Input, tea.Cmd)
	View() string
	Focus() tea.Cmd
	SetValue(string)
}

type ShortAnswerField struct {
	textinput textinput.Model
}

func (sa *ShortAnswerField) Value() string {
	return sa.textinput.Value()
}

func (sa *ShortAnswerField) Blur() tea.Msg {
	return sa.textinput.Blur
}

func (sa *ShortAnswerField) Focus() tea.Cmd {
	return sa.textinput.Focus()
}

func (sa *ShortAnswerField) SetValue(val string) {
	sa.textinput.SetValue(val)
}

func (sa *ShortAnswerField) Update(msg tea.Msg) (Input, tea.Cmd) {
	var cmd tea.Cmd
	sa.textinput, cmd = sa.textinput.Update(msg)
	return sa, cmd
}

func (sa *ShortAnswerField) View() string {
	return sa.textinput.View()
}

type LongAnswerField struct {
	textarea textarea.Model
}

func (la *LongAnswerField) Value() string {
	return la.textarea.Value()
}

func (la *LongAnswerField) Blur() tea.Msg {
	return la.textarea.Blur
}

func (la *LongAnswerField) Focus() tea.Cmd {
	return la.textarea.Focus()
}

func (la *LongAnswerField) SetValue(val string) {
	la.textarea.SetValue(val)
}

func (la *LongAnswerField) Update(msg tea.Msg) (Input, tea.Cmd) {
	var cmd tea.Cmd
	la.textarea, cmd = la.textarea.Update(msg)
	return la, cmd
}

func (la *LongAnswerField) View() string {
	return la.textarea.View()
}

func newLongAnswerField() *LongAnswerField {
	ta := textarea.New()
	ta.SetWidth(maxWidth)
	ta.Placeholder = "Type your answer here..."
	ta.ShowLineNumbers = false
	ta.Focus()
	return &LongAnswerField{ta}
}

func newShortAnswerField() *ShortAnswerField {
	ti := textinput.New()
	ti.Placeholder = "Type your answer here..."
	ti.Width = maxWidth
	ti.Focus()
	return &ShortAnswerField{ti}
}

type Question struct {
	text   string
	answer string
	input  Input
}

func newQuestion(text string, isLong bool) Question {
	var input Input
	if isLong {
		input = newLongAnswerField()
	} else {
		input = newShortAnswerField()
	}
	return Question{
		text:  text,
		input: input,
	}
}

type QuestionSet struct {
	title        string
	instructions string
	questions    []Question
}

type styles struct {
	BorderColor lipgloss.Color
	InputField  lipgloss.Style
}

func defaultStyles() *styles {
	s := new(styles)
	s.BorderColor = lipgloss.Color("45")
	s.InputField = lipgloss.NewStyle().
		BorderForeground(s.BorderColor).
		BorderStyle(lipgloss.NormalBorder()).
		Padding(1).
		Width(maxWidth)
	return s
}

// registration info
type registrationInfo struct {
	name  string
	level string
	id    string
	email string
}

type model struct {
	progress progress.Model
	viewport viewport.Model
	styles   *styles

	questionSet        QuestionSet
	registration       registrationInfo
	registrationFields []Input

	currentQuestion int
	screen          screen
	registrationIdx int
	hasSavedState   bool
	restoreChoice   int
	restoreProgress bool
}

func initialModel() model {
	regFields := []Input{
		newShortAnswerField(),
		newShortAnswerField(),
		newShortAnswerField(),
		newShortAnswerField(),
	}

	questions := []Question{
		newQuestion("What command would you use to rename a directory called \\cd_backup to \\cd?", false),
		newQuestion("Write a shell script that takes a directory path as an argument, and renames all images in the directory to the format \"img_yymmdd\". The script must include: \n 1- A shebang\n 2- A command line argument parser \n 3- 3 functions.", true),
		newQuestion("How can we use the find command to rename all files larger than 5Mb to all caps?", false),
	}

	questionSet := QuestionSet{
		title:        "Moving and Renaming",
		instructions: "Write or paste your solution, then click ctrl+s to submit and go to the next question.",
		questions:    questions,
	}

	p := progress.New(progress.WithGradient("#"+hexBlue, "#"+hexLightBlue))
	v := viewport.New(maxWidth, 4)

	m := model{
		progress:           p,
		viewport:           v,
		questionSet:        questionSet,
		registrationFields: regFields,
		//screen:             registrationScreen,
		//registrationIdx:    0,
		styles: defaultStyles(),
	}

	if data, err := os.ReadFile(stateFile); err == nil {
		var state PersistedState
		if json.Unmarshal(data, &state) == nil && !state.Completed {
			m.hasSavedState = true
			m.screen = restoreScreen
		} else {
			_ = os.Remove(stateFile)
			m.screen = registrationScreen
		}
	} else {
		m.screen = registrationScreen
	}

	return m
}

func (m model) Init() tea.Cmd {
	if m.screen == questionScreen && m.currentQuestion >= 0 {
		return m.questionSet.questions[m.currentQuestion].input.Focus()
	}
	if len(m.registrationFields) > 0 {
		return m.registrationFields[m.registrationIdx].Focus()
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	if m.screen == restoreScreen {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyUp:
				if m.restoreChoice > 0 {
					m.restoreChoice--
				}
			case tea.KeyDown:
				if m.restoreChoice < 1 {
					m.restoreChoice++
				}
			case tea.KeyEnter:
				if m.restoreChoice == 0 {
					m = loadState(m)
				} else {
					m = resumeSession(m)
					m.screen = registrationScreen
				}
			}
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			saveState(m)
			return m, tea.Quit

		case tea.KeyEnter, tea.KeyTab:
			if m.screen == registrationScreen {
				if m.registrationIdx < len(m.registrationFields)-1 {
					m.registrationFields[m.registrationIdx].Blur()
					m.registrationIdx++
					cmd = m.registrationFields[m.registrationIdx].Focus()
					cmds = append(cmds, cmd)
				} else {
					// save registration
					m.registration.name = m.registrationFields[0].Value()
					m.registration.level = m.registrationFields[1].Value()
					m.registration.id = m.registrationFields[2].Value()
					m.registration.email = m.registrationFields[3].Value()

					saveState(m)

					log.Printf("Registration: name=%s, level=%s, id=%s, email=%s",
						m.registration.name, m.registration.level,
						m.registration.id, m.registration.email)

					// switch to questions
					m.screen = questionScreen
					if len(m.questionSet.questions) > 0 {
						cmd = m.questionSet.questions[0].input.Focus()
						cmds = append(cmds, cmd)
					}
				}
			}

		case tea.KeyCtrlS:
			if m.screen == questionScreen {
				if m.currentQuestion != -1 && m.currentQuestion < len(m.questionSet.questions) {
					current := &m.questionSet.questions[m.currentQuestion]
					current.answer = current.input.Value()

					log.Printf("Question %d: %q -> %q",
						m.currentQuestion+1, current.text, current.answer)

					if m.currentQuestion < len(m.questionSet.questions)-1 {
						m.currentQuestion++
						cmd = m.questionSet.questions[m.currentQuestion].input.Focus()
						cmds = append(cmds, cmd)
					} else {
						m.currentQuestion = -1
						log.Printf("All questions completed.")
					}

					progressValue := float64(m.currentQuestion) / float64(len(m.questionSet.questions))
					if m.currentQuestion == -1 {
						progressValue = 1.0
					}
					progressCmd := m.progress.SetPercent(progressValue)
					cmds = append(cmds, progressCmd)
				}
			}
			saveState(m)
		}

	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height / 4
	}

	if m.screen == registrationScreen && m.registrationIdx < len(m.registrationFields) {
		var inputCmd tea.Cmd
		m.registrationFields[m.registrationIdx], inputCmd = m.registrationFields[m.registrationIdx].Update(msg)
		cmds = append(cmds, inputCmd)
	} else if m.screen == questionScreen && m.currentQuestion != -1 && m.currentQuestion < len(m.questionSet.questions) {
		current := m.questionSet.questions[m.currentQuestion]
		var inputCmd tea.Cmd
		current.input, inputCmd = current.input.Update(msg)
		m.questionSet.questions[m.currentQuestion] = current
		cmds = append(cmds, inputCmd)
	}

	if m.restoreProgress {
		answered := m.currentQuestion
		if answered < 0 {
			answered = len(m.questionSet.questions)
		}

		m.progress.SetPercent(
			float64(answered) / float64(len(m.questionSet.questions)),
		)

		m.restoreProgress = false
	}

	progressModel, cmd := m.progress.Update(msg)
	m.progress = progressModel.(progress.Model)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	switch m.screen {
	case restoreScreen:
		return m.restoreView()
	case registrationScreen:
		return m.registrationView()
	case questionScreen:
		return m.questionView()
	default:
		return "Unknown screen"
	}
}

func (m model) registrationView() string {
	labels := []string{"Name:", "Level:", "ID:", "Email:"}

	var fields []string
	for i, field := range m.registrationFields {
		prefix := "  "
		if i == m.registrationIdx {
			prefix = "> "
		}
		fields = append(fields, fmt.Sprintf("%s%s %s", prefix, labels[i], field.View()))
	}

	help := "• Tab/Enter: next field • Enter on last field: continue to questions\n• ctrl+c: quit"

	return containerStyle.Render(lipgloss.JoinVertical(lipgloss.Top,
		boldStyle.Render("Registration"),
		"\n",
		subtleStyle.Render("Please enter your information:"),
		"\n",
		lipgloss.JoinVertical(lipgloss.Left, fields...),
		"\n",
		subtleStyle.Render(help),
	))
}

func (m model) questionView() string {
	if m.currentQuestion == -1 {
		help := "• ctrl+c: quit"
		return containerStyle.Render(
			lipgloss.JoinVertical(lipgloss.Top,
				boldStyle.Render(m.questionSet.title),
				"\n",
				"All done! Thank you for completing the questions.",
				"\n",
				m.progress.View(),
				"\n",
				help))
	} else {
		current := m.questionSet.questions[m.currentQuestion]
		prompt := subtleStyle.Render("Answer:")
		help := "• ctrl+s: submit and go to next question\n• ctrl+c: quit"

		m.viewport.SetContent(questionStyle.Render(current.text))
		inputView := m.styles.InputField.Render(current.input.View())

		return containerStyle.Render(lipgloss.JoinVertical(lipgloss.Top,
			boldStyle.Render(m.questionSet.title),
			"\n",
			subtleStyle.Render(fmt.Sprintf("Question %d of %d",
				m.currentQuestion+1, len(m.questionSet.questions))),
			m.viewport.View(),
			"\n",
			subtleStyle.Render(prompt),
			inputView,
			"\n",
			m.progress.View(),
			"\n",
			subtleStyle.Render(help),
		))
	}
}

func StartTUI() error {

	f, err := tea.LogToFile("debug.log", "DBG: ")
	if err != nil {
		return err
	}
	defer f.Close()

	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	_, err = p.Run()
	return err
}
