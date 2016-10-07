package logging

type Logger interface {
	Info(...interface{})
	Error(...interface{})
	Fatal(...interface{})
	Infof(string, ...interface{})
	Errorf(string, ...interface{})
	Fatalf(string, ...interface{})
}