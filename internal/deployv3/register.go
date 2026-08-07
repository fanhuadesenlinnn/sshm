package deployv3

func init() {
	Register(commandModule{name: "command"})
	Register(commandModule{name: "shell"})
	Register(&fileModule{})
	Register(&copyModule{})
	Register(&templateModule{})
	Register(&serviceModule{})
	Register(&waitForModule{})
	Register(&failModule{})
	Register(&debugModule{})
	Register(&unarchiveModule{})
	Register(&fetchModule{})
	Register(&pauseModule{})
}
