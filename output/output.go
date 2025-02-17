package output

type Output interface {
}

type service struct {
}

func NewOutputService() Output {
	return service{}
}
