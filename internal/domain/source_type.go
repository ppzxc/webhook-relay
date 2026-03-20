package domain

type SourceType string

const (
	SourceTypeBeszel  SourceType = "BESZEL"
	SourceTypeDozzle  SourceType = "DOZZLE"
	SourceTypeGeneric SourceType = "GENERIC"
)

func (s SourceType) IsValid() bool {
	switch s {
	case SourceTypeBeszel, SourceTypeDozzle, SourceTypeGeneric:
		return true
	}
	return false
}
