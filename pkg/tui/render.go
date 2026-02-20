// package tui

// import (
// 	"fmt"

// 	"github.com/charmbracelet/bubbles/progress"
// 	"github.com/charmbracelet/bubbles/textarea"
// 	"github.com/charmbracelet/bubbles/viewport"
// 	tea "github.com/charmbracelet/bubbletea"
// 	"github.com/charmbracelet/lipgloss"
// )

// // styling and formatting
// const (
// 	maxWidth            = 50
// 	hexBlue      string = "68b0f4"
// 	hexLightBlue string = "bddfff"
// )

// var (
// 	containerStyle = lipgloss.NewStyle().
// 			Margin(2)
// 	boldStyle = lipgloss.NewStyle().
// 			Foreground(lipgloss.Color("#" + hexBlue)).
// 			Bold(true)
// 	subtleStyle = lipgloss.NewStyle().
// 			Foreground(lipgloss.Color("#" + hexLightBlue))
// 	borderStyle = lipgloss.NewStyle().
// 			Foreground(lipgloss.Color("#" + hexBlue)).
// 			Border(lipgloss.RoundedBorder())
// 	questionStyle = lipgloss.NewStyle().Width(maxWidth)
// )

// // question handling
// type Question struct {
// 	text   string
// 	answer string
// }
// type QuestionSet struct {
// 	title        string
// 	instructions string
// 	questions    []Question
// }

// // bubbletea

// type model struct {
// 	textarea        textarea.Model
// 	progress        progress.Model
// 	viewport        viewport.Model
// 	questionSet     QuestionSet
// 	currentQuestion int
// }

// func initialModel() model {
// 	questionSet := QuestionSet{
// 		title:        "Moving and Renaming",
// 		instructions: "Write or paste your solution, then click ctrl+s to submit and go to the next question.",
// 		questions: []Question{
// 			{text: "What command would you use to rename a directory called \\cd_backup to \\cd?", answer: ""},
// 			{text: "Write a shell script that takes a directory path as an argument, and renames all images in the directory to the format \"img_yymmdd\". The script must be blah blah blah and include: \n 1- A shebang\n 2- A command line argument parser \n 3- 3 functions. l script that takes a directory path as an argument, and renames all images in the directory to the format \"img_yymmdd\". The script must be blah blah blah and include: \n 1- A shebang\n 2- A command line argument parser \n l script that takes a directory path as an argument, and renames all images in the directory to the format \"img_yymmdd\". The script must be blah blah blah and include: \n 1- A shebang\n 2- A command line argument parser \n ", answer: ""},
// 			{text: "How can we use the find command to rename all files larger than 5Mb to all caps?", answer: ""}},
// 	}

// 	t := textarea.New()
// 	t.SetWidth(maxWidth)
// 	t.ShowLineNumbers = false
// 	t.Focus()

// 	p := progress.New(progress.WithGradient("#"+hexBlue, "#"+hexLightBlue))
// 	v := viewport.New(maxWidth, 4)
// 	return model{progress: p, textarea: t, viewport: v, questionSet: questionSet, currentQuestion: 0}
// }

// func (m model) Init() tea.Cmd {
// 	return textarea.Blink
// }

// func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
// 	var cmds []tea.Cmd
// 	var cmd tea.Cmd

// 	switch msg := msg.(type) {
// 	case tea.KeyMsg:
// 		switch msg.Type {
// 		case tea.KeyEsc:
// 			if m.textarea.Focused() {
// 				m.textarea.Blur()
// 			}
// 		case tea.KeyCtrlC:
// 			return m, tea.Quit

// 		case tea.KeyCtrlS:
// 			if m.currentQuestion != -1 {
// 				m.questionSet.questions[m.currentQuestion].answer = m.textarea.Value()
// 				if m.currentQuestion < len(m.questionSet.questions)-1 {
// 					m.currentQuestion++
// 					m.textarea.Reset()
// 				} else {
// 					m.currentQuestion = -1
// 				}
// 			}
// 			cmd = m.progress.IncrPercent(float64(1) / float64(len(m.questionSet.questions)))
// 			cmds = append(cmds, cmd)

// 		default:
// 			if !m.textarea.Focused() {
// 				cmd = m.textarea.Focus()
// 				cmds = append(cmds, cmd)
// 			}
// 		}
// 	case progress.FrameMsg:
// 		progressModel, cmd := m.progress.Update(msg)
// 		m.progress = progressModel.(progress.Model)
// 		return m, cmd
// 	}
// 	m.textarea, cmd = m.textarea.Update(msg)
// 	cmds = append(cmds, cmd)

// 	m.viewport, cmd = m.viewport.Update(msg)

// 	cmds = append(cmds, cmd)
// 	return m, tea.Batch(cmds...)
// }

// func (m model) View() string {

// 	if m.currentQuestion == -1 {
// 		help := "• ctrl+c: quit"
// 		return containerStyle.Render(
// 			lipgloss.JoinVertical(lipgloss.Top,
// 				boldStyle.Render(m.questionSet.title),
// 				"\n",
// 				"All done!",
// 				"\n",
// 				m.progress.View(),
// 				"\n",
// 				help))
// 	} else {
// 		prompt := subtleStyle.Render("Answer:")
// 		help := "• ctrl+s: submit and go to next question\n• ctrl+c: quit"
// 		m.viewport.SetContent(questionStyle.Render(m.questionSet.questions[m.currentQuestion].text))
// 		return containerStyle.Render(lipgloss.JoinVertical(lipgloss.Top,
// 			boldStyle.Render(m.questionSet.title),
// 			"\n",
// 			subtleStyle.Render(fmt.Sprintf("Question %d", m.currentQuestion+1)),
// 			m.viewport.View(),
// 			"\n",
// 			subtleStyle.Render(prompt),
// 			borderStyle.Render(m.textarea.View()),
// 			"\n",
// 			m.progress.View(),
// 			"\n",
// 			subtleStyle.Render(help),
// 		))
// 	}
// }

// func StartTUI() error {
// 	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
// 	if _, err := p.Run(); err != nil {
// 		return err
// 	} else {
// 		return nil
// 	}

// }
package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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

// screen types
type screen int

const (
	registrationScreen screen = iota
	questionScreen
)

// Input interface for different answer field types
type Input interface {
	Value() string
	Blur() tea.Msg
	Update(tea.Msg) (Input, tea.Cmd)
	View() string
	Focus() tea.Cmd
	SetValue(string)
}

// ShortAnswerField implements Input for short text answers
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

// LongAnswerField implements Input for long text answers
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

// factory functions
func newLongAnswerField() *LongAnswerField {
	ta := textarea.New()
	ta.SetWidth(maxWidth)
	ta.Placeholder = "Type your answer here... (ctrl+s to submit)"
	ta.ShowLineNumbers = false
	ta.Focus()
	return &LongAnswerField{ta}
}

func newShortAnswerField() *ShortAnswerField {
	ti := textinput.New()
	ti.Placeholder = "Type your answer here... (ctrl+s to submit)"
	ti.Width = maxWidth
	ti.Focus()
	return &ShortAnswerField{ti}
}

// Question
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

// Styles
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

// Registration info
type registrationInfo struct {
	name  string
	level string
	id    string
	email string
}

// main model
type model struct {
	// UI Components
	progress progress.Model
	viewport viewport.Model
	styles   *styles

	// Data
	questionSet        QuestionSet
	registration       registrationInfo
	registrationFields []Input

	// State
	currentQuestion int
	screen          screen
	registrationIdx int
}

func initialModel() model {
	// Registration fields
	regFields := []Input{
		newShortAnswerField(),
		newShortAnswerField(),
		newShortAnswerField(),
		newShortAnswerField(),
	}

	// Questions
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

	return model{
		progress:           p,
		viewport:           v,
		questionSet:        questionSet,
		registrationFields: regFields,
		screen:             registrationScreen,
		registrationIdx:    0,
		styles:             defaultStyles(),
	}
}

func (m model) Init() tea.Cmd {
	if len(m.registrationFields) > 0 {
		return m.registrationFields[0].Focus()
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyEnter, tea.KeyTab:
			if m.screen == registrationScreen {
				if m.registrationIdx < len(m.registrationFields)-1 {
					m.registrationFields[m.registrationIdx].Blur()
					m.registrationIdx++
					cmd = m.registrationFields[m.registrationIdx].Focus()
					cmds = append(cmds, cmd)
				} else {
					// Save registration
					m.registration.name = m.registrationFields[0].Value()
					m.registration.level = m.registrationFields[1].Value()
					m.registration.id = m.registrationFields[2].Value()
					m.registration.email = m.registrationFields[3].Value()

					// Optional: log to file if desired; here we ignore
					// Switch to questions
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

					if m.currentQuestion < len(m.questionSet.questions)-1 {
						m.currentQuestion++
						cmd = m.questionSet.questions[m.currentQuestion].input.Focus()
						cmds = append(cmds, cmd)
					} else {
						m.currentQuestion = -1
					}

					progressValue := float64(m.currentQuestion+1) / float64(len(m.questionSet.questions))
					if m.currentQuestion == -1 {
						progressValue = 1.0
					}
					progressCmd := m.progress.SetPercent(progressValue)
					cmds = append(cmds, progressCmd)
				}
			}
		}

	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
	}

	// Update current input
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

	// Update progress (generic)
	progressModel, cmd := m.progress.Update(msg)
	m.progress = progressModel.(progress.Model)
	cmds = append(cmds, cmd)

	// Update viewport
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	switch m.screen {
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

// Run starts the TUI application. It returns an error if the program fails.
func Run() error {
	// Optional: set up logging to a file for debugging
	// f, err := tea.LogToFile("debug.log", "DBG: ")
	// if err != nil {
	// 	return err
	// }
	// defer f.Close()

	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
