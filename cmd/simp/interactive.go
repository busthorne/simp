package main

import (
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

var interactiveHelp = `Interactive mode assumes multiline input: ctrl+d to submit, ctrl+c to quit.`

type multilineModel struct {
	pending []byte
	cancel  bool
}

func (m multilineModel) Init() tea.Cmd {
	return nil
}

func (m multilineModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlD:
			return m, tea.Quit
		case tea.KeyCtrlC:
			m.cancel = true
			return m, tea.Quit
		case tea.KeyRunes:
			s := ""
			for _, r := range msg.Runes {
				s += string(r)
			}
			m.pending = append(m.pending, s...)
		case tea.KeyEnter:
			m.pending = append(m.pending, '\n')
		case tea.KeyBackspace:
			if len(m.pending) > 0 {
				m.pending = m.pending[:len(m.pending)-1]
			}
		}
	}
	return m, nil
}

func (m multilineModel) View() string {
	return string(m.pending)
}

func multiline() (string, error) {
	p := tea.NewProgram(multilineModel{})
	model, err := p.Run()
	if err != nil {
		return "", err
	}
	m := model.(multilineModel)
	if m.cancel || len(m.pending) == 0 {
		return "", io.EOF
	}
	s := string(m.pending)
	fmt.Println(s)
	return s, nil
}
