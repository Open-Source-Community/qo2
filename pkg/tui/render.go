package tui

import (
	"fmt"
	"log"
	"strconv"

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

type QuestionSet struct {
	title        string
	instructions string
	questions    []Question
}

// questions screen

type questionModel struct {
	textarea        textarea.Model
	progress        progress.Model
	viewport        viewport.Model
	Session 		*Session
	questionSet     *QuestionSet
	currentQuestion int
}

func initialQuestionModel(user *User) questionModel {
	session:= InitializeSession(user)

	questionSet := session.questionsSet
	if len(questionSet.questions) == 0{
		log.Fatal("Failed to fetch questions!")
	}

	t := textarea.New()
	t.SetWidth(maxWidth)
	t.ShowLineNumbers = false
	t.Focus()

	p := progress.New(progress.WithGradient("#"+hexBlue, "#"+hexLightBlue))
	v := viewport.New(maxWidth, 4)
	return questionModel{progress: p, textarea: t, viewport: v, questionSet: questionSet, currentQuestion: 0}
}

func (m questionModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m questionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m questionModel) View() string {

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

// info form screen


type infoModel struct {
	inputs          []textinput.Model
	focused         int
	progress        progress.Model
	done            bool
	user            *User
}

func InitialInfoModel() infoModel {
	inputs := make([]textinput.Model, 5)
	
	inputs[0] = textinput.New()
	inputs[0].Placeholder = "Full Name"
	inputs[0].Focus()

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "Email Address"

	inputs[2] = textinput.New()
	inputs[2].Placeholder = "Phone Number"

	inputs[3] = textinput.New()
	inputs[3].Placeholder = "Year (1-4)"

	inputs[4] = textinput.New()
	inputs[4].Placeholder = "Are you an OSC member? (y/n)"

	p := progress.New(progress.WithGradient("#"+hexBlue, "#"+hexLightBlue))

	return infoModel{
		inputs:   inputs,
		focused:  0,
		progress: p,
		user:     &User{},
	}
}

func (m infoModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m infoModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit

		case tea.KeyEnter, tea.KeyTab:
			if m.focused == len(m.inputs)-1 {
				m.done = true
				m.confirmUserInfo()
				return initialQuestionModel(m.user), nil
			}

			m.focused++
			// Progress bar calculation
			progCmd := m.progress.SetPercent(float64(m.focused) / float64(len(m.inputs)))
			
			// Focus the next input
			for i := range m.inputs {
				m.inputs[i].Blur()
			}
			m.inputs[m.focused].Focus()
			
			return m, progCmd

		case tea.KeyShiftTab:
			if m.focused > 0 {
				m.focused--
				for i := range m.inputs {
					m.inputs[i].Blur()
				}
				m.inputs[m.focused].Focus()
			}
		}
	
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	}

	// Update the currently focused input
	cmd := m.updateInputs(msg)
	return m, cmd
}

func (m *infoModel) updateInputs(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
	return cmd
}

func (m *infoModel) confirmUserInfo() {
	m.user.name = m.inputs[0].Value()
	m.user.email = m.inputs[1].Value()
	m.user.phone = m.inputs[2].Value()
	m.user.year, _ = strconv.Atoi(m.inputs[3].Value())
	m.user.oscian = m.inputs[4].Value() == "y" || m.inputs[4].Value() == "yes"
}
 

func (m infoModel) View() string {
	if m.done {
		return containerStyle.Render(fmt.Sprintf(
			"%s\n\nProfile Created for %s!",
			boldStyle.Render(""),
			m.user.name,
		))
	}

	var labels = []string{"Name", "Email", "Phone", "Academic Year", "Oscian?"}

	return containerStyle.Render(lipgloss.JoinVertical(lipgloss.Top,
		boldStyle.Render("Just a few questions before we start!"),
		"\n",
		subtleStyle.Render(labels[m.focused]),
		borderStyle.Render(m.inputs[m.focused].View()),
		"\n",
		m.progress.View(),
		"\n",
		subtleStyle.Render("• enter: next field • ctrl+c: quit"),
	))
}
// main model



func StartTUI() error {
	p := tea.NewProgram(InitialInfoModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	} else {
		return nil
	}

}
