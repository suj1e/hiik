package api

// Notification levels.
const (
	// LevelBlocked means an agent stopped and waits for a person.
	LevelBlocked = "blocked"
	// LevelDone means an agent finished a turn.
	LevelDone = "done"
	// LevelInfo is any other message.
	LevelInfo = "info"
)

// Notification is one message the app should bring to the user, such as an
// agent that became blocked or finished a turn while the user looked
// elsewhere. Core builds it; the shell decides how to show it.
type Notification struct {
	// Title is the short line, usually the agent's name.
	Title string
	// Body is the detail shown after the title, if any.
	Body string
	// Level is LevelBlocked, LevelDone or LevelInfo.
	Level string
	// Terminal is the agent's core address, when the message is about one.
	Terminal string
	// Sound asks for the notification tone. Core sets it from the sound
	// setting when it delivers the notification.
	Sound bool
}
