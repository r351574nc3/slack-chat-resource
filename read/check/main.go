package main

import (
	"encoding/json"
	"fmt"
	"github.com/cludden/slack-chat-resource/utils"
	"github.com/nlopes/slack"
	"os"
)

func main() {
	var request utils.CheckRequest

	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		fatal("Parsing request.", err)
	}

	if len(request.Source.Token) == 0 {
		fatal1("Missing source field: token.")
	}

	if len(request.Source.ChannelID) == 0 {
		fatal1("Missing source field: channel_id.")
	}

	if request.Source.Filter != nil {
		fmt.Fprintf(os.Stderr, "Filter:\n")
		fmt.Fprintf(os.Stderr, "  - author: %s\n", request.Source.Filter.AuthorID)
		fmt.Fprintf(os.Stderr, "  - pattern: %s\n", request.Source.Filter.TextPattern)
	}

	if request.Source.ReplyFilter != nil {
		fmt.Fprintf(os.Stderr, "Reply Filter:\n")
		fmt.Fprintf(os.Stderr, "  - author: %s\n", request.Source.ReplyFilter.AuthorID)
		fmt.Fprintf(os.Stderr, "  - pattern: %s\n", request.Source.ReplyFilter.TextPattern)
	}

	client := slack.New(request.Source.Token)
	history := getMessages(&request, client)
	versions := []utils.Version{}

	for _, msg := range history.Messages {
		accept, stop := processMessage(&msg, &request, client)

		if accept {
			version := utils.Version{"timestamp": msg.Msg.Timestamp}
			versions = append(versions, version)
		}

		if stop {
			break
		}
	}

	response := utils.CheckResponse{}
	for i := len(versions) - 1; i >= 0; i-- {
		response = append(response, versions[i])
	}

	json.NewEncoder(os.Stdout).Encode(&response)
}

// Channel type definition
type Channel struct {
	id   string
	name string
}

// ChannelsMeta type definition
type ChannelsMeta struct {
	nextCursor string
}

// Channels type definition
type Channels struct {
	ok       bool
	channels []Channel
	meta     ChannelsMeta
}

func getMessages(request *utils.CheckRequest, client *slack.Client) *slack.History {
	params := slack.NewHistoryParameters()

	if version, ok := request.Version["timestamp"]; ok {
		params.Oldest = version
		fmt.Fprintf(os.Stderr, "Request timestamp: %s\n", version)
	}

	params.Inclusive = true
	params.Count = 100

	var history *slack.History
	history, err := client.GetChannelHistory(request.Source.ChannelID, params)
	if err != nil {
		fatal("getting messages", err)
	}

	return history
}

func processMessage(message *slack.Message, request *utils.CheckRequest,
	client *slack.Client) (accept bool, stop bool) {

	isReply := len(message.Msg.ThreadTimestamp) > 0 &&
		message.Msg.ThreadTimestamp != message.Msg.Timestamp

	if isReply {
		fmt.Fprintf(os.Stderr, "Message %s is a reply. Skipping.\n", message.Msg.Timestamp)
		return false, false
	}

	fmt.Fprintf(os.Stderr, "- Message %s: %s \n", message.Msg.Timestamp, message.Msg.Text)

	if request.Source.Filter != nil {
		fmt.Fprintf(os.Stderr, "Matching message...\n")
		if !matchMessage(message, request.Source.Filter) {
			fmt.Fprintf(os.Stderr, "Message did not matched.\n")
			return false, false
		}
	}

	if request.Source.ReplyFilter != nil {
		fmt.Fprintf(os.Stderr, "Matching replies...\n")
		if matchReplies(message, request, client) {
			fmt.Fprintf(os.Stderr, "A reply was matched.\n")
			return false, true
		}
	}

	return true, false
}

func matchMessage(message *slack.Message, filter *utils.MessageFilter) bool {
	authorID := filter.AuthorID
	if len(authorID) > 0 && message.Msg.User != authorID && message.Msg.BotID != authorID {
		fmt.Fprintf(os.Stderr, "Author is not %s.\n", authorID)
		return false
	}

	pattern := filter.TextPattern
	if pattern != nil && !pattern.MatchString(message.Msg.Text) {
		fmt.Fprintf(os.Stderr, "Message text does not match pattern.\n")
		return false
	}

	fmt.Fprintf(os.Stderr, "Message matched.\n")
	return true
}

func matchReplies(message *slack.Message, request *utils.CheckRequest, client *slack.Client) bool {
	if message.Msg.ReplyCount == 0 {
		return false
	}

	replies, err := client.GetChannelReplies(request.Source.ChannelID, message.Msg.Timestamp)
	if err != nil {
		fatal("getting replies", err)
	}

	for _, reply := range replies[1:] {
		fmt.Fprintf(os.Stderr, "- A reply: %s\n", reply.Msg.Text)
		if matchMessage(&reply, request.Source.ReplyFilter) {
			return true
		}
	}

	return false
}

func fatal(doing string, err error) {
	fmt.Fprintf(os.Stderr, "error "+doing+": "+err.Error()+"\n")
	os.Exit(1)
}

func fatal1(reason string) {
	fmt.Fprintf(os.Stderr, reason+"\n")
	os.Exit(1)
}
