package tui

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	overlay "github.com/floatpane/bubble-overlay"
)

// constants & styles

const (
	maxWidth            = 60
	hexBlue      string = "68b0f4"
	hexLightBlue string = "bddfff"
	hexGreen     string = "77dd77"
	hexRed       string = "f26e6e"
)

var (
	containerStyle = lipgloss.NewStyle().Margin(2).Align(lipgloss.Center)
	boldStyle      = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#" + hexBlue)).
			Bold(true)
	subtleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#" + hexLightBlue))
	borderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#" + hexBlue)).
			Border(lipgloss.RoundedBorder())
	questionStyle = lipgloss.NewStyle().Width(maxWidth)
	cardStyle     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#"+hexLightBlue)).
			Padding(1, 2).
			MarginTop(1).
			Align(lipgloss.Center)
	outputPassStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#"+hexGreen)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#"+hexGreen)).
			Width(maxWidth).
			Padding(0, 1)
	outputFailStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#"+hexRed)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#"+hexRed)).
			Width(maxWidth).
			Padding(0, 1)
	popupStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#"+hexBlue)).
			Foreground(lipgloss.Color("#"+hexLightBlue)).
			Border(lipgloss.RoundedBorder()).
			Padding(1, 3)

	windowHeight = 0
	windowWidth  = 0
)

type QuestionSet struct {
	title        string
	instructions string
	questions    []*Question
}

// tea messages

// gradingDoneMsg is sent back to the model when async grading finishes.
type gradingDoneMsg struct {
	index  int
	output string
	pass   bool
	err    error
}

// question screen

type questionModel struct {
	textarea         textarea.Model
	progress         progress.Model
	questionViewport viewport.Model
	outputViewport   viewport.Model
	spinner          spinner.Model
	session          *Session
	questionSet      *QuestionSet
	currentQuestion  int
	grading          bool // true while waiting for sandbox result
	showOutput       bool // true after grading completes, before next question
	lastOutput       string
	lastPass         bool
	popup            bool
}

func initialQuestionModel(user *User) questionModel {
	session, err := InitializeSession(user)
	if err != nil {
		log.Fatalf("Failed to initialize session: %v", err)
	}
	questionSet := session.questionSet
	if len(questionSet.questions) == 0 {
		log.Fatal("Failed to fetch questions!")
	}

	t := textarea.New()
	t.SetWidth(maxWidth)
	t.ShowLineNumbers = false
	t.Focus()

	p := progress.New(progress.WithGradient("#"+hexBlue, "#"+hexLightBlue))
	qv := viewport.New(maxWidth, 5)
	ov := viewport.New(maxWidth, 6)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#" + hexBlue))

	return questionModel{
		progress:         p,
		textarea:         t,
		questionViewport: qv,
		outputViewport:   ov,
		spinner:          sp,
		questionSet:      questionSet,
		currentQuestion:  0,
		session:          session,
	}
}

func (m questionModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m questionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	numQuestions := len(m.questionSet.questions)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		windowHeight = msg.Height
		windowWidth = msg.Width

	case gradingDoneMsg:
		m.grading = false
		m.showOutput = true
		if msg.err != nil {
			m.lastOutput = msg.output + "\n" + msg.err.Error()
			m.lastPass = false
		} else {
			m.lastOutput = msg.output
			m.lastPass = msg.pass
		}
		m.outputViewport.SetContent(m.lastOutput)

		// advance progress bar
		cmd = m.progress.IncrPercent(float64(1) / float64(numQuestions))
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		if m.currentQuestion == -1 {
			if m.session != nil {
				m.session.SaveSession()
			}
			return m, tea.Quit

		} else {
			switch msg.Type {

			case tea.KeyEsc:
				if m.textarea.Focused() {
					m.textarea.Blur()
				}
			case tea.KeyCtrlE:
				q := m.session.questionSet.questions[m.currentQuestion]

				if m.grading {
					return m, nil
				}
				// block re-runs for oneShot
				if q.oneShot && q.attempted {
					m.lastOutput = "This question allows only one attempt."
					m.lastPass = false
					m.showOutput = true
					m.outputViewport.SetContent(m.lastOutput)
					return m, nil
				}
				m.showOutput = false
				m.textarea.Blur()
				//m.grading = true

				// record the answer and kick off async grading
				q.answer = strings.TrimSpace(m.textarea.Value())
				if q.answer != "" {
					q.attempted = true
					m.grading = true

					cmds = append(cmds, gradeAnswer(m.session, m.currentQuestion))
					cmds = append(cmds, m.spinner.Tick)
					return m, tea.Batch(cmds...)
				}

			case tea.KeyCtrlRight:
				if !m.lastPass {
					cmds = append(cmds, gradeAnswer(m.session, m.currentQuestion))
				}
				m.currentQuestion = (m.currentQuestion + 1) % numQuestions
				m.lastOutput = ""
				m.lastPass = false
				m.grading = false
				m.showOutput = false
				m.textarea.Reset()
				return m, nil
			case tea.KeyCtrlLeft:
				if !m.lastPass {
					cmds = append(cmds, gradeAnswer(m.session, m.currentQuestion))
				}
				m.currentQuestion = (m.currentQuestion - 1 + numQuestions) % numQuestions
				m.lastOutput = ""
				m.lastPass = false
				m.grading = false
				m.showOutput = false
				m.textarea.Reset()
				return m, nil

			case tea.KeyCtrlS, tea.KeyCtrlC:
				m.popup = true
			case tea.KeyEnter:
				if m.popup {
					m.session.SaveSession()
					m.currentQuestion = -1
				}

			default:
				//if user presses any key other than enter -> go back and don't submit
				if m.popup {
					m.popup = false
				}
				// if output is shown and user presses anything -> go back to editing
				if m.showOutput && !m.grading {
					switch msg.String() {
					case "up", "down":
						// let these slip to allow scrolling!!!
					default:
						m.showOutput = false
						cmd = m.textarea.Focus()
						cmds = append(cmds, cmd)
						return m, tea.Batch(cmds...)
					}
				}

				if !m.textarea.Focused() && !m.grading {
					cmd = m.textarea.Focus()
					cmds = append(cmds, cmd)
				}
			}
		}

	case spinner.TickMsg:
		if m.grading {
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	}

	if !m.grading {
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
		m.questionViewport, cmd = m.questionViewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	if m.showOutput {
		m.outputViewport, cmd = m.outputViewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m questionModel) View() string {
	if windowHeight == 0 { // wait for WindowSizeMsg to reach
		return "Initializing..."
	}
	// finished screen
	if m.currentQuestion == -1 {
		content := lipgloss.JoinVertical(lipgloss.Center,
			boldStyle.Render("Great job!"),
			"",
			"You're all done!",
			//fmt.Sprintf("Final Result: %s", boldStyle.Render(m.session.result)),
			"",
			//m.progress.View(),
			"",
			subtleStyle.Render("Press any key to exit."),
		)
		return containerStyle.Render(cardStyle.Align(lipgloss.Center).Render(content))
	}

	// question screen
	q := m.questionSet.questions[m.currentQuestion]

	m.questionViewport.SetContent(questionStyle.Render(q.text))

	// Format Title like: "Topic Name ### x"
	attemptTag := ""
	if m.session.questionSet.questions[m.currentQuestion].oneShot {
		attemptTag = " x"
	}
	titleStr := fmt.Sprintf("Question %d: %s", m.currentQuestion+1, attemptTag)

	parts := []string{
		boldStyle.Render(titleStr),
		"",
		m.questionViewport.View(),
		"",
	}

	if m.grading {
		parts = append(parts,
			lipgloss.JoinHorizontal(lipgloss.Left,
				m.spinner.View(),
				"  ",
				subtleStyle.Render("Running in sandbox…"),
			),
		)
	} else if m.showOutput {
		label := "Output"
		outputBox := outputPassStyle
		if !m.lastPass {
			outputBox = outputFailStyle
			label = "Output   "
		} else {
			label = "Output   "
		}

		parts = append(parts,
			subtleStyle.Render(label),
			outputBox.Render(m.outputViewport.View()),
			"",
		)
	} else {
		parts = append(parts,
			subtleStyle.Render("Answer:"),
			borderStyle.Render(m.textarea.View()),
			"",
			//m.progress.View(),
			"",
		)
	}
	popup := popupStyle.Render("Press <enter> to submit\nor any key to cancel")
	base := containerStyle.Render(lipgloss.JoinVertical(lipgloss.Top, parts...))
	if m.popup {
		return overlay.Center(base, popup, windowWidth, windowHeight)
	} else {
		return base
	}
}

// info form screen

type infoModel struct {
	inputs   []textinput.Model
	focused  int
	progress progress.Model
	done     bool
	user     *User
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
	case tea.WindowSizeMsg:
		windowHeight = msg.Height
		windowWidth = msg.Width
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
			progCmd := m.progress.SetPercent(float64(m.focused) / float64(len(m.inputs)))
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

	labels := []string{"Name", "Email", "Phone", "Academic Year", "Oscian?"}

	return containerStyle.Render(lipgloss.JoinVertical(lipgloss.Top,
		boldStyle.Render("Just a few questions before we start!"),
		"",
		subtleStyle.Render(labels[m.focused]),
		borderStyle.Render(m.inputs[m.focused].View()),
		"",
		m.progress.View(),
		"",
		subtleStyle.Render("enter: next field  •  ctrl+c: quit"),
	))
}

// entry point

func StartTUI() error {
	p := tea.NewProgram(InitialInfoModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func gradeAnswer(session *Session, index int) tea.Cmd {
	return func() tea.Msg {
		q := session.questionSet.questions[index]
		output, err := q.gradeWithSandbox(session.sandbox)
		session.SubmitQuestion(index)
		return gradingDoneMsg{
			index:  index,
			output: output,
			pass:   q.score == 1,
			err:    err,
		}
	}
}
