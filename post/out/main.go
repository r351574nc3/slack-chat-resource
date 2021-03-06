package main

import (
	"encoding/json"
	"fmt"
	"github.com/cludden/slack-chat-resource/utils"
	"github.com/nlopes/slack"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		println("usage: " + os.Args[0] + " <source>")
		os.Exit(1)
	}

	sourceDir := os.Args[1]

	var request utils.OutRequest
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		fatal("Parsing request.", err)
	}

	if len(request.Source.Token) == 0 {
		fatal1("Missing source field: token.")
	}

	if len(request.Source.ChannelID) == 0 {
		fatal1("Missing source field: channel_id.")
	}

	if len(request.Params.MessageFile) == 0 && request.Params.Message == nil {
		fatal1("Missing params field: message or message_file.")
	}

	var message *utils.OutMessage
	if len(request.Params.MessageFile) != 0 {
		message = new(utils.OutMessage)
		readMessageFile(filepath.Join(sourceDir, request.Params.MessageFile), message)
	} else {
		message = request.Params.Message
		interpolateMessage(message, sourceDir)
	}

	fmt.Fprintf(os.Stderr, "Sending message:\n")
	m, _ := json.MarshalIndent(message, "", "  ")
	fmt.Fprintf(os.Stderr, "%s\n", m)

	client := slack.New(request.Source.Token)
	response := send(message, &request, client)
	if err := json.NewEncoder(os.Stdout).Encode(&response); err != nil {
		fatal("encoding response", err)
	}
}

func readMessageFile(path string, message *utils.OutMessage) {
	file, err := os.Open(path)
	if err != nil {
		fatal("opening message file", err)
	}

	if err := json.NewDecoder(file).Decode(message); err != nil {
		fatal("reading message file", err)
	}
}

func interpolateMessage(message *utils.OutMessage, sourceDir string) {
	message.Text = interpolate(message.Text, sourceDir)
	message.ThreadTimestamp = interpolate(message.ThreadTimestamp, sourceDir)

	for i, a := range message.Attachments {
		message.Attachments[i] = interpolateMessageAttachment(a, sourceDir)
	}

	for i, b := range message.Blocks.BlockSet {
		message.Blocks.BlockSet[i] = interpolateMessageBlock(b, sourceDir)
	}
}

func interpolateMessageAttachment(attachment slack.Attachment, sourceDir string) slack.Attachment {
	attachment.Fallback = interpolate(attachment.Fallback, sourceDir)
	attachment.Title = interpolate(attachment.Title, sourceDir)
	attachment.TitleLink = interpolate(attachment.TitleLink, sourceDir)
	attachment.Pretext = interpolate(attachment.Pretext, sourceDir)
	attachment.Text = interpolate(attachment.Text, sourceDir)
	attachment.Footer = interpolate(attachment.Footer, sourceDir)
	return attachment
}

func interpolateMessageBlock(block slack.Block, sourceDir string) slack.Block {
	switch block.BlockType() {
	case slack.MBTAction:
		b := block.(*slack.ActionBlock)
		for i, e := range b.Elements.ElementSet {
			b.Elements.ElementSet[i] = interpolateMessageBlockElement(e, sourceDir)
		}
		return b
	case slack.MBTContext:
		b := block.(*slack.ContextBlock)
		for i, e := range b.ContextElements.Elements {
			b.ContextElements.Elements[i] = interpolateMessageMixedElement(e, sourceDir)
		}
		return b
	case slack.MBTSection:
		b := block.(*slack.SectionBlock)
		b.Text = interpolateTextBlock(b.Text, sourceDir)
		for i, e := range b.Fields {
			b.Fields[i] = interpolateTextBlock(e, sourceDir)
		}
		return b
	}
	return block
}

func interpolateMessageBlockElement(elem slack.BlockElement, sourceDir string) slack.BlockElement {
	switch elem.ElementType() {
	case slack.METButton:
		e := elem.(*slack.ButtonBlockElement)
		e.Text.Text = interpolate(e.Text.Text, sourceDir)
		return e
	}
	return elem
}

func interpolateMessageMixedElement(elem slack.MixedElement, sourceDir string) slack.MixedElement {
	switch elem.MixedElementType() {
	case slack.MixedElementText:
		e := elem.(*slack.TextBlockObject)
		e.Text = interpolate(e.Text, sourceDir)
		return e
	}
	return elem
}

func interpolateTextBlock(b *slack.TextBlockObject, sourceDir string) *slack.TextBlockObject {
	b.Text = interpolate(b.Text, sourceDir)
	return b
}

func getFileContents(path string) string {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		fatal("reading file", err)
	}
	return strings.TrimSpace(string(data))
}

func interpolate(text string, sourceDir string) string {
	var out string

	start := 0
	end := 0
	inside := false
	c0 := '_'

	for pos, c1 := range text {
		if inside {
			if c0 == '}' && c1 == '}' {
				inside = false
				end = pos + 1

				var value string

				if text[start+2] == '$' {
					target := text[start+3 : end-2]
					value = os.Getenv(target)
				} else {
					target := text[start+2 : end-2]
					value = getFileContents(filepath.Join(sourceDir, target))
				}

				out += value
			}
		} else {
			if c0 == '{' && c1 == '{' {
				inside = true
				start = pos - 1
				out += text[end:start]
			}
		}
		c0 = c1
	}

	out += text[end:]

	return out
}

func send(message *utils.OutMessage, request *utils.OutRequest, client *slack.Client) utils.OutResponse {
	opts := []slack.MsgOption{
		slack.MsgOptionPostMessageParameters(message.PostMessageParameters),
	}
	if message.Text != "" {
		opts = append(opts, slack.MsgOptionText(message.Text, false))
	}
	if len(message.Attachments) > 0 {
		opts = append(opts, slack.MsgOptionAttachments(message.Attachments...))
	}
	if len(message.Blocks.BlockSet) > 0 {
		opts = append(opts, slack.MsgOptionBlocks(message.Blocks.BlockSet...))
	}

	_, timestamp, err := client.PostMessage(request.Source.ChannelID, opts...)

	if err != nil {
		fatal("sending", err)
	}

	var response utils.OutResponse
	response.Version = utils.Version{"timestamp": timestamp}
	return response
}

func fatal(doing string, err error) {
	fmt.Fprintf(os.Stderr, "Error "+doing+": "+err.Error()+"\n")
	os.Exit(1)
}

func fatal1(reason string) {
	fmt.Fprintf(os.Stderr, reason+"\n")
	os.Exit(1)
}
