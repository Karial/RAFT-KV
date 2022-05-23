package src

type Command string

type RSM interface {
	Apply(Command) (string, error)
}
