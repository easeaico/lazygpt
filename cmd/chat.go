/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sashabaranov/go-openai"
	"github.com/spf13/cobra"
)

type chatUI struct {
	textarea    textarea.Model
	err         error
	senderStyle lipgloss.Style
	viewport    viewport.Model
	messages    []string
}

func initChatUI() *chatUI {
	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Focus()

	ta.Prompt = "┃ "
	ta.CharLimit = 280

	ta.SetWidth(30)
	ta.SetHeight(3)

	// Remove cursor line styling
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	ta.ShowLineNumbers = false

	vp := viewport.New(300, 50)
	vp.SetContent(fmt.Sprintf(`Welcome to the ChatGPT(%s)!
Type a message and press Enter to send.`, conf.Model))

	ta.KeyMap.InsertNewline.SetEnabled(false)

	return &chatUI{
		textarea:    ta,
		messages:    []string{},
		viewport:    vp,
		senderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		err:         nil,
	}
}

func (c chatUI) Init() tea.Cmd {
	return textarea.Blink
}

func (c chatUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	c.textarea, tiCmd = c.textarea.Update(msg)
	c.viewport, vpCmd = c.viewport.Update(msg)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			fmt.Println(c.textarea.Value())
			return c, tea.Quit
		case tea.KeyEnter:
			input := c.textarea.Value()
			if input == "/exit" {
				return c, tea.Quit
			}

			messages := []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "You are a helpful assistant.",
				},
			}

			var role string
			for i, msg := range c.messages {
				if i%2 == 0 {
					role = openai.ChatMessageRoleUser
				} else {
					role = openai.ChatMessageRoleAssistant
				}
				chatMsg := openai.ChatCompletionMessage{
					Role:    role,
					Content: msg,
				}
				messages = append(messages, chatMsg)

			}
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: input,
			})

			c.messages = append(c.messages, c.senderStyle.Render("You: ")+input)
			c.viewport.SetContent(strings.Join(c.messages, "\n"))
			c.textarea.Reset()
			c.viewport.GotoBottom()

			resp, err := client.CreateChatCompletion(
				context.Background(),
				openai.ChatCompletionRequest{
					Model:    openai.GPT3Dot5Turbo,
					Messages: messages,
				},
			)
			if err != nil {
				c.err = fmt.Errorf("ChatCompletion error: %w", err)
				return c, nil
			}

			c.messages = append(c.messages, c.senderStyle.Render("AI: ")+resp.Choices[0].Message.Content)
			c.viewport.SetContent(strings.Join(c.messages, "\n"))
			c.textarea.Reset()
			c.viewport.GotoBottom()
		}

	// We handle errors just like any other message
	case error:
		c.err = msg
		return c, nil
	}

	return c, tea.Batch(tiCmd, vpCmd)
}

func (m chatUI) View() string {
	return fmt.Sprintf(
		"%s\n\n%s",
		m.viewport.View(),
		m.textarea.View(),
	) + "\n\n"
}

type Config struct {
	Endpoint string `toml:"endpoint"`
	APIKey   string `toml:"api_key"`
	Model    string `toml:"model"`
}

var (
	conf   Config
	client *openai.Client
	ui     *chatUI
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

	client = openai.NewClient(conf.APIKey)
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
		p := tea.NewProgram(initChatUI())

		if _, err := p.Run(); err != nil {
			log.Fatal(err)
		}
	},
}
