package types

// QuestionRequest is a clarifying question posed to the user by the
// AskUserQuestion tool (BF-08), delivered over the HIL channel pair the same
// way permission requests flow (permissions.AskRequest precedent).
type QuestionRequest struct {
	// ID correlates the response with the request.
	ID string
	// Question is the text to display.
	Question string
	// Options are optional suggested answers (numbered in the TUI).
	Options []string
}

// QuestionResponse carries the user's answer back to the waiting tool.
type QuestionResponse struct {
	// ID matches the QuestionRequest.
	ID string
	// Answer is the user's reply (free text or a chosen option).
	Answer string
	// Dismissed is true when the user pressed Esc instead of answering.
	Dismissed bool
}
