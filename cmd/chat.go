/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sashabaranov/go-openai"
	"github.com/spf13/cobra"
)

type resultMsg struct {
	data  string // message recevied
	index int    // message index number
}

type model struct {
	textarea    textarea.Model
	err         error
	respChan    chan resultMsg
	senderStyle lipgloss.Style
	messageList list.Model
	spinner     spinner.Model
}

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type message struct {
	role string // 消息发送者的 j角色
	desc string // 消息内容
}

func (m message) Role() string { return m.role }
func (m message) Title() string {
	switch m.role {
	case openai.ChatMessageRoleAssistant:
		return "🤖: "
	case openai.ChatMessageRoleUser:
		return "👤: "
	default:
		return ""
	}
}
func (m message) Description() string { return m.desc }
func (m message) FilterValue() string {
	return m.desc
}

func initialModel(respChan chan resultMsg) *model {
	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Focus()

	ta.Prompt = "┃ "
	ta.CharLimit = 2800

	ta.SetWidth(400)
	ta.SetHeight(4)

	// Remove cursor line styling
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false)

	msgList := list.New([]list.Item{}, list.NewDefaultDelegate(), 400, 60)
	msgList.Title = fmt.Sprintf("Model: %s", conf.Model)

	return &model{
		textarea:    ta,
		messageList: msgList,
		senderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		err:         nil,
		respChan:    respChan,
	}
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	m.textarea, tiCmd = m.textarea.Update(msg)
	m.messageList, vpCmd = m.messageList.Update(msg)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			fmt.Println(m.textarea.Value())
			return m, tea.Quit
		case tea.KeyCtrlY:
			msg := m.messageList.SelectedItem().(message)
			err := clipboard.WriteAll(msg.Description())
			if err != nil {
				m.err = err
				slog.Error("update error", slog.Any("error", m.err))
				return m, nil
			}

			return m, nil
		case tea.KeyEnter:
			input := m.textarea.Value()
			//darkMsg, err := glamour.Render(input, "dark")
			//if err != nil {
			//	m.err = err
			//	slog.Error("markdown render error", slog.Any("error", m.err))
			//	return m, nil
			//}

			end := len(m.messageList.Items())
			go m.streamRequest(input, m.respChan, end+1)
			m.textarea.Reset()

			return m, tea.Batch(m.messageList.InsertItem(end, message{
				role: openai.ChatMessageRoleUser,
				desc: input,
			}), m.messageList.InsertItem(end+1, message{
				role: openai.ChatMessageRoleAssistant,
				desc: "",
			}))
		}
	case resultMsg:
		return m, m.messageList.SetItem(msg.index, message{
			role: openai.ChatMessageRoleAssistant,
			desc: msg.data,
		})
	case error:
		m.err = msg
		slog.Error("update error", slog.Any("error", m.err))
		return m, nil
	}

	return m, tea.Batch(tiCmd, vpCmd)
}

func (m *model) streamRequest(input string, respChan chan resultMsg, index int) {
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: "You are a helpful assistant.",
		},
	}

	for _, msg := range m.messageList.Items() {
		ite := msg.(message)
		chatMsg := openai.ChatCompletionMessage{
			Role:    ite.role,
			Content: ite.desc,
		}
		messages = append(messages, chatMsg)
	}
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: input,
	})

	stream, err := client.CreateChatCompletionStream(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:    conf.Model,
			Messages: messages,
		},
	)
	if err != nil {
		m.err = fmt.Errorf("chat completion error: %w", err)
		slog.Error("chat completion error", slog.Any("error", m.err))
		return
	}

	var content strings.Builder
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			m.err = err
			slog.Error("stream recv error", slog.Any("error", m.err))
			return
		}

		content.WriteString(resp.Choices[0].Delta.Content)

		//darkMsg, err := glamour.Render(content.String(), "dark")
		//if err != nil {
		//	m.err = err
		//	slog.Error("markdown render error", slog.Any("error", m.err))
		//	return
		//}
		respChan <- resultMsg{
			index: index,
			data:  content.String(),
		}
	}
}

func (m model) View() string {
	return fmt.Sprintf(
		"%s\n\n%s",
		docStyle.Render(m.messageList.View()),
		m.textarea.View(),
	) + "\n\n"
}

type Config struct {
	ApiUrl string `toml:"api_url"`
	ApiKey string `toml:"api_key"`
	Model  string `toml:"model"`
}

var (
	conf   Config
	client *openai.Client
	m      *model
)

func init() {
	rootCmd.AddCommand(chatCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// chatCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// chatCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	tomlData, err := os.ReadFile("config.toml")
	if err != nil {
		log.Fatalf("Error reading YAML file: %s", err)
	}

	err = toml.Unmarshal(tomlData, &conf)
	if err != nil {
		log.Fatalf("Error reading YAML file: %s", err)
	}

	config := openai.DefaultConfig(conf.ApiKey)
	config.BaseURL = conf.ApiUrl
	client = openai.NewClientWithConfig(config)
}

// chatCmd represents the chat command
var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		respChan := make(chan resultMsg, 10)
		quitChan := make(chan struct{}, 1)
		p := tea.NewProgram(initialModel(respChan), tea.WithAltScreen())
		go func() {
			for {
				select {
				case msg := <-respChan:
					p.Send(msg)
				case <-quitChan:
					return
				}
			}
		}()
		if _, err := p.Run(); err != nil {
			log.Fatal(err)
		}
		quitChan <- struct{}{}
	},
}
