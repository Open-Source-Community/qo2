package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textarea"
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

// question handling
type Question struct {
	text   string
	answer string
}
type QuestionSet struct {
	title        string
	instructions string
	questions    []Question
}

// bubbletea

type model struct {
	textarea        textarea.Model
	progress        progress.Model
	viewport        viewport.Model
	questionSet     QuestionSet
	currentQuestion int
}

func initialModel() model {
	questionSet := QuestionSet{
		title:        "Moving and Renaming",
		instructions: "Write or paste your solution, then click ctrl+s to submit and go to the next question.",
		questions: []Question{
			{text: "What command would you use to rename a directory called \\cd_backup to \\cd?", answer: ""},
			{text: "Write a shell script that takes a directory path as an argument, and renames all images in the directory to the format \"img_yymmdd\". The script must be blah blah blah and include: \n 1- A shebang\n 2- A command line argument parser \n 3- 3 functions. l script that takes a directory path as an argument, and renames all images in the directory to the format \"img_yymmdd\". The script must be blah blah blah and include: \n 1- A shebang\n 2- A command line argument parser \n l script that takes a directory path as an argument, and renames all images in the directory to the format \"img_yymmdd\". The script must be blah blah blah and include: \n 1- A shebang\n 2- A command line argument parser \n ", answer: ""},
			{text: "How can we use the find command to rename all files larger than 5Mb to all caps?", answer: ""}},
	}

	t := textarea.New()
	t.SetWidth(maxWidth)
	t.ShowLineNumbers = false
	t.Focus()

	p := progress.New(progress.WithGradient("#"+hexBlue, "#"+hexLightBlue))
	v := viewport.New(maxWidth, 4)
	return model{progress: p, textarea: t, viewport: v, questionSet: questionSet, currentQuestion: 0}
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			if m.textarea.Focused() {
				m.textarea.Blur()
			}
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyCtrlS:
			if m.currentQuestion != -1 {
				m.questionSet.questions[m.currentQuestion].answer = m.textarea.Value()
				if m.currentQuestion < len(m.questionSet.questions)-1 {
					m.currentQuestion++
					m.textarea.Reset()
				} else {
					m.currentQuestion = -1
				}
			}
			cmd = m.progress.IncrPercent(float64(1) / float64(len(m.questionSet.questions)))
			cmds = append(cmds, cmd)

		default:
			if !m.textarea.Focused() {
				cmd = m.textarea.Focus()
				cmds = append(cmds, cmd)
			}
		}
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	}
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)

	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m model) View() string {

	if m.currentQuestion == -1 {
		help := "• ctrl+c: quit"
		return containerStyle.Render(
			lipgloss.JoinVertical(lipgloss.Top,
				boldStyle.Render(m.questionSet.title),
				"\n",
				"All done!",
				"\n",
				m.progress.View(),
				"\n",
				help))
	} else {
		prompt := subtleStyle.Render("Answer:")
		help := "• ctrl+s: submit and go to next question\n• ctrl+c: quit"
		m.viewport.SetContent(questionStyle.Render(m.questionSet.questions[m.currentQuestion].text))
		return containerStyle.Render(lipgloss.JoinVertical(lipgloss.Top,
			boldStyle.Render(m.questionSet.title),
			"\n",
			subtleStyle.Render(fmt.Sprintf("Question %d", m.currentQuestion+1)),
			m.viewport.View(),
			"\n",
			subtleStyle.Render(prompt),
			borderStyle.Render(m.textarea.View()),
			"\n",
			m.progress.View(),
			"\n",
			subtleStyle.Render(help),
		))
	}
}

func StartTUI() error {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	} else {
		return nil
	}

}
