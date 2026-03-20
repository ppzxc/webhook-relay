package domain

type InputType string

const (
	InputTypeBeszel  InputType = "BESZEL"
	InputTypeDozzle  InputType = "DOZZLE"
	InputTypeGeneric InputType = "GENERIC"
)

func (s InputType) IsValid() bool {
	switch s {
	case InputTypeBeszel, InputTypeDozzle, InputTypeGeneric:
		return true
	}
	return false
}
