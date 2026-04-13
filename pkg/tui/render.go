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
)

// constants & styles

const (
	maxWidth            = 60
	hexBlue      string = "68b0f4"
	hexLightBlue string = "bddfff"
	hexYellow    string = "FFFF00"
	hexRed       string = "f26e6e"
)

var (
	containerStyle = lipgloss.NewStyle().Margin(2)
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
			MarginTop(1)
	outputPassStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#"+hexYellow)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#"+hexYellow)).
			Width(maxWidth).
			Padding(0, 1)
	outputFailStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#"+hexRed)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#"+hexRed)).
			Width(maxWidth).
			Padding(0, 1)
)

type QuestionSet struct {
	title        string
	instructions string
	topics       []string
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
	grading          bool // true while waiting for sandbox result
	showOutput       bool // true after grading completes, before next question
	lastOutput       string
	lastPass         bool
}

func initialQuestionModel(user *User, config SessionConfig) questionModel {
	session, err := InitializeSession(user, config)
	if err != nil {
		log.Fatalf("Failed to initialize session: %v", err)
	}
	questionSet := session.questionSet
	if len(questionSet.questions) == 0 {
		log.Fatal("Failed to fetch questions!")
	}
	session.currentQuestion = 0

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
		session:          session,
	}
}

func (m questionModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m questionModel) skipTopic() (tea.Model, tea.Cmd) {
	if q, err := m.session.GetCurrentQuestion(); err == nil && q != nil {
		q.score = 0
		m.session.SubmitAnswer("SKIPPED TOPIC")
	}
	_ = m.session.AdvanceTopic(false)
	m.session.AdvanceQuestion()
	m.textarea.Reset()
	m.showOutput = false
	return m, nil
}

func (m questionModel) skipDifficulty() (tea.Model, tea.Cmd) {
	if q, err := m.session.GetCurrentQuestion(); err == nil && q != nil {
		q.score = 0
		m.session.SubmitAnswer("SKIPPED DIFFICULTY")
	}
	_ = m.session.IncreaseDifficulty(false)
	m.session.AdvanceQuestion()
	m.textarea.Reset()
	m.showOutput = false
	return m, nil
}

func (m questionModel) skipQuestion() (tea.Model, tea.Cmd) {
	if q, err := m.session.GetCurrentQuestion(); err == nil && q != nil {
		q.score = 0
		m.session.SubmitAnswer("SKIPPED")
	}
	m.session.AdvanceQuestion()
	m.textarea.Reset()
	m.showOutput = false
	return m, nil
}

func (m questionModel) finishSession() (tea.Model, tea.Cmd) {
	m.session.currentQuestion = -1
	m.session.Finalize()
	return m, nil
}

func (m questionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {

	case gradingDoneMsg:
		m.grading = false
		m.showOutput = true
		if msg.err != nil {
			m.lastOutput = fmt.Sprintf("grading error: %v", msg.err)
			m.lastPass = false
		} else {
			m.lastOutput = strings.TrimSpace(msg.output)
			if m.lastOutput == "" {
				m.lastOutput = "(no output)"
			}
			m.lastPass = msg.pass
		}
		m.outputViewport.SetContent(m.lastOutput)

		// advance progress bar
		cmd = m.progress.IncrPercent(float64(1) / float64(len(m.session.questionSet.questions)))
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlG:
			if !m.grading {
				return m.skipTopic()
			}
		case tea.KeyCtrlD:
			if !m.grading {
				return m.skipDifficulty()
			}
		case tea.KeyCtrlN:
			if !m.grading {
				return m.skipQuestion()
			}
		case tea.KeyCtrlF:
			if !m.grading {
				return m.finishSession()
			}

		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyEsc:
			if m.textarea.Focused() {
				m.textarea.Blur()
			}

		// interviewer controls
		case tea.KeyShiftLeft:
			m.session.AdvanceTopic(true)
		case tea.KeyShiftRight:
			m.session.AdvanceTopic(false)
		case tea.KeyShiftUp:
			m.session.IncreaseDifficulty(false)
		case tea.KeyShiftDown:
			m.session.IncreaseDifficulty(true)

		// run
		case tea.KeyCtrlE:
			if m.grading {
				return m, nil
			}

			q := m.session.questionSet.questions[m.session.currentQuestion]

			// run start_up script for each question as soon as loaded
			if !q.attempted && q.setup_script != "" {
				_, err := m.session.sandbox.Run(q.setup_script)
				//fmt.Println("setup out:", out, "err:", err)
				if err != nil {
					m.lastOutput = "setup failed: " + err.Error()
					m.lastPass = false
					m.showOutput = true
					return m, nil
				}
			}

			// block re-runs for oneShot
			if q.oneShot && q.attempted {
				m.lastOutput = "This question allows only one attempt."
				m.lastPass = false
				m.showOutput = true
				m.outputViewport.SetContent(m.lastOutput)
				return m, nil
			}
			m.grading = true
			m.showOutput = false
			m.textarea.Blur()

			q.answer = m.textarea.Value()
			q.attempted = true
			//m.grading = true

			// record the answer and kick off async grading
			m.session.questionSet.questions[m.session.currentQuestion].answer = m.textarea.Value()
			m.grading = true

			cmds = append(cmds, runCommandAsync(m.session, m.session.currentQuestion))
			cmds = append(cmds, m.spinner.Tick)
			return m, tea.Batch(cmds...)

		case tea.KeyCtrlS:
			// only allow advancing if output is shown
			if !m.showOutput {
				return m, nil
			}
			q, _ := m.session.GetCurrentQuestion()

			if !q.attempted {
				return m, nil
			}
			m.showOutput = false

			// run test script to auto grade then immediately run clean_up script
			oldQ, _ := m.session.GetCurrentQuestion()
			if oldQ.test_script != "" {
				status, err := m.session.sandbox.Run(oldQ.test_script)
				if err != nil {
					//fmt.Println(status, err)
					oldQ.score = 0
				} else if strings.TrimSpace(status) == "ok" {
					//fmt.Println(status, err)
					if oldQ.difficulty == 1 {
						oldQ.score = 1
					} else if oldQ.difficulty == 2 {
						oldQ.score = 3
					} else {
						oldQ.score = 5
					}
				} else {
					//fmt.Println(status, err)
					oldQ.score = 0
				}
			}
			if oldQ.cleanup_script != "" {
				m.session.sandbox.Run(oldQ.cleanup_script)
			}
			m.session.SubmitAnswer(q.answer)

			m.session.AdvanceQuestion()

			m.textarea.Reset()
			return m, nil
		default:
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
	// finished screen
	if m.session.currentQuestion == -1 {
		content := lipgloss.JoinVertical(lipgloss.Center,
			boldStyle.Render("Great job!"),
			"",
			"You're all done!",
			//fmt.Sprintf("Final Result: %s", boldStyle.Render(m.session.result)),
			"",
			//m.progress.View(),
			"",
			subtleStyle.Render("press ctrl+c to exit"),
		)
		return containerStyle.Render(cardStyle.Align(lipgloss.Center).Render(content))
	}

	// question screen
	q, _ := m.session.GetCurrentQuestion()

	m.questionViewport.SetContent(questionStyle.Render(q.text))

	// Format Title like: "Topic Name ### x"
	diffLvl := m.session.currentDifficulty
	if diffLvl < 1 { diffLvl = 1 } // just in case
	diffTags := strings.Repeat("#", diffLvl)
	attemptTag := ""
	if m.session.questionSet.questions[m.session.currentQuestion].oneShot {
		attemptTag = " x"
	}
	titleStr := fmt.Sprintf("%s %s%s", m.session.GetCurrentTopic(), diffTags, attemptTag)

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

		// displayOutput := strings.TrimSpace(m.lastOutput)
		// if displayOutput == "" {
		// 	displayOutput = "(no output)"
		// }
		// m.outputViewport.SetContent(displayOutput)

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

	return containerStyle.Render(lipgloss.JoinVertical(lipgloss.Top, parts...))
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
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit

		case tea.KeyEnter, tea.KeyTab:
			if m.focused == len(m.inputs)-1 {
				m.done = true
				m.confirmUserInfo()
				return initialSelectionModel(m.user), nil
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
		subtleStyle.Render("enter: next field  •  ctrl+c: quit tool completely"),
	))
}

// selection screen

type selectionModel struct {
	user                 *User
	topics               []string
	maxTopicCounts       map[string]int
	selectedTopicCounts  map[string]int
	diffs                []int
	maxDiffCounts        map[int]int
	selectedDiffCounts   map[int]int
	globalSelectedTopics map[string]bool
	globalTotal          int
	mode                 SelectionMode
	cursor               int
	input                textinput.Model
	isEditing            bool
}

func initialSelectionModel(user *User) tea.Model {
	topicCounts, err := FetchTopicsWithCounts()
	if err != nil {
		log.Fatalf("failed to fetch topics: %v", err)
	}

	diffCounts, err := FetchDifficultyWithCounts()
	if err != nil {
		log.Fatalf("failed to fetch difficulties: %v", err)
	}

	var topics []string
	for t := range topicCounts {
		topics = append(topics, t)
	}

	var diffs = []int{1, 2, 3}

	ti := textinput.New()
	ti.Placeholder = "Enter number"
	ti.CharLimit = 3
	ti.Width = 10

	return selectionModel{
		user:                 user,
		topics:               topics,
		maxTopicCounts:       topicCounts,
		selectedTopicCounts:  make(map[string]int),
		diffs:                diffs,
		maxDiffCounts:        diffCounts,
		selectedDiffCounts:   make(map[int]int),
		globalSelectedTopics: make(map[string]bool),
		globalTotal:          0,
		mode:                 ModeByTopic,
		input:                ti,
	}
}

func (m selectionModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m selectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.isEditing {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter:
				val, _ := strconv.Atoi(m.input.Value())
				if val < 0 {
					val = 0
				}
				switch m.mode {
				case ModeByTopic:
					topic := m.topics[m.cursor]
					if val > m.maxTopicCounts[topic] {
						val = m.maxTopicCounts[topic]
					}
					m.selectedTopicCounts[topic] = val
				case ModeGlobal:
					m.globalTotal = val
				case ModeByDifficulty:
					diff := m.diffs[m.cursor]
					if val > m.maxDiffCounts[diff] {
						val = m.maxDiffCounts[diff]
					}
					m.selectedDiffCounts[diff] = val
				}
				m.isEditing = false
				m.input.Blur()
				return m, nil
			case tea.KeyEsc:
				m.isEditing = false
				m.input.Blur()
				return m, nil
			}
		}
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyTab:
			m.mode = (m.mode + 1) % 3
			m.cursor = 0
			return m, nil
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			max := 0
			switch m.mode {
			case ModeByTopic:
				max = len(m.topics) - 1
			case ModeGlobal:
				max = len(m.topics) // total input + topics
			case ModeByDifficulty:
				max = len(m.diffs) - 1
			}
			if m.cursor < max {
				m.cursor++
			}
		case tea.KeyEnter, tea.KeySpace:
			switch m.mode {
			case ModeByTopic:
				m.isEditing = true
				topic := m.topics[m.cursor]
				m.input.SetValue(strconv.Itoa(m.selectedTopicCounts[topic]))
				m.input.Focus()
			case ModeGlobal:
				if m.cursor == 0 {
					m.isEditing = true
					m.input.SetValue(strconv.Itoa(m.globalTotal))
					m.input.Focus()
				} else {
					topic := m.topics[m.cursor-1]
					m.globalSelectedTopics[topic] = !m.globalSelectedTopics[topic]
				}
			case ModeByDifficulty:
				m.isEditing = true
				diff := m.diffs[m.cursor]
				m.input.SetValue(strconv.Itoa(m.selectedDiffCounts[diff]))
				m.input.Focus()
			}
			return m, nil
		case tea.KeyCtrlS:
			config := SessionConfig{Mode: m.mode}
			hasSelection := false

			switch m.mode {
			case ModeByTopic:
				config.TopicCounts = make(map[string]int)
				for topic, count := range m.selectedTopicCounts {
					if count > 0 {
						config.TopicCounts[topic] = count
						hasSelection = true
					}
				}
			case ModeGlobal:
				config.GlobalCount = m.globalTotal
				for topic, selected := range m.globalSelectedTopics {
					if selected {
						config.GlobalTopics = append(config.GlobalTopics, topic)
						hasSelection = true
					}
				}
				if m.globalTotal <= 0 {
					hasSelection = false
				}
			case ModeByDifficulty:
				config.DifficultyCounts = make(map[int]int)
				for diff, count := range m.selectedDiffCounts {
					if count > 0 {
						config.DifficultyCounts[diff] = count
						hasSelection = true
					}
				}
			}

			if !hasSelection {
				return m, nil
			}
			return initialQuestionModel(m.user, config), nil
		}
	}

	return m, nil
}

func (m selectionModel) View() string {
	s := boldStyle.Render("Configure your session") + "\n\n"

	// Mode Selector
	modes := []string{"By Topic", "Global", "By Difficulty"}
	modeBar := ""
	for i, name := range modes {
		if int(m.mode) == i {
			modeBar += "[" + boldStyle.Render(name) + "] "
		} else {
			modeBar += name + "  "
		}
	}
	s += modeBar + "\n\n"

	switch m.mode {
	case ModeByTopic:
		s += subtleStyle.Render("Set number of questions per topic:") + "\n\n"
		for i, topic := range m.topics {
			cursor := " "
			if m.cursor == i {
				cursor = ">"
			}
			count := m.selectedTopicCounts[topic]
			max := m.maxTopicCounts[topic]
			topicStr := fmt.Sprintf("%-20s", topic)
			countStr := fmt.Sprintf("[%d/%d]", count, max)

			if m.cursor == i {
				if m.isEditing {
					s += fmt.Sprintf("%s %s %s\n", boldStyle.Render(cursor), topicStr, m.input.View())
				} else {
					s += fmt.Sprintf("%s %s %s (Enter to edit)\n", boldStyle.Render(cursor), topicStr, boldStyle.Render(countStr))
				}
			} else {
				s += fmt.Sprintf("  %s %s\n", topicStr, countStr)
			}
		}

	case ModeGlobal:
		s += subtleStyle.Render("Enter total questions and select topics:") + "\n\n"
		// Total input
		cur := " "
		if m.cursor == 0 {
			cur = ">"
		}
		if m.cursor == 0 && m.isEditing {
			s += fmt.Sprintf("%s Total Questions: %s\n\n", boldStyle.Render(cur), m.input.View())
		} else {
			disp := fmt.Sprintf("Total Questions: %d", m.globalTotal)
			if m.cursor == 0 {
				s += fmt.Sprintf("%s %s (Enter to edit)\n\n", boldStyle.Render(cur), boldStyle.Render(disp))
			} else {
				s += fmt.Sprintf("  %s\n\n", disp)
			}
		}

		for i, topic := range m.topics {
			cur = " "
			if m.cursor == i+1 {
				cur = ">"
			}
			checked := "[ ]"
			if m.globalSelectedTopics[topic] {
				checked = "[x]"
			}
			topicStr := fmt.Sprintf("%s %s", checked, topic)
			if m.cursor == i+1 {
				s += fmt.Sprintf("%s %s\n", boldStyle.Render(cur), boldStyle.Render(topicStr))
			} else {
				s += fmt.Sprintf("  %s\n", topicStr)
			}
		}

	case ModeByDifficulty:
		s += subtleStyle.Render("Set number of questions per difficulty:") + "\n\n"
		for i, diff := range m.diffs {
			cursor := " "
			if m.cursor == i {
				cursor = ">"
			}
			count := m.selectedDiffCounts[diff]
			max := m.maxDiffCounts[diff]
			diffStr := fmt.Sprintf("Level %d", diff)
			countStr := fmt.Sprintf("[%d/%d]", count, max)

			if m.cursor == i {
				if m.isEditing {
					s += fmt.Sprintf("%s %-20s %s\n", boldStyle.Render(cursor), diffStr, m.input.View())
				} else {
					s += fmt.Sprintf("%s %-20s %s (Enter to edit)\n", boldStyle.Render(cursor), diffStr, boldStyle.Render(countStr))
				}
			} else {
				s += fmt.Sprintf("  %-20s %s\n", diffStr, countStr)
			}
		}
	}

	s += "\n" + subtleStyle.Render("↑/↓: navigate • tab: switch mode • enter/space: select/edit • ctrl+s: start")
	return containerStyle.Render(s)
}

// entry point

func StartTUI() error {
	p := tea.NewProgram(InitialInfoModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
func runCommandAsync(session *Session, index int) tea.Cmd {
	return func() tea.Msg {
		q := session.questionSet.questions[index]

		if session.sandbox == nil {
			return gradingDoneMsg{
				index:  index,
				output: "sandbox not initialized",
				pass:   false,
				err:    fmt.Errorf("no sandbox"),
			}
		}

		output, err := session.sandbox.Run(q.answer)

		return gradingDoneMsg{
			index:  index,
			output: output,
			pass:   err == nil,
			err:    err,
		}
	}
}
