package endpoint

//go:generate err4gen .

func then(err *error, ok func(), catch func()) {
	switch {
	case *err == nil && ok != nil:
		ok()
	case *err != nil && catch != nil:
		catch()
	}
}
