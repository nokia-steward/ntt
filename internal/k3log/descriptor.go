package k3log

type Descriptor struct {
	// IsError marks an error event
	IsError bool `yaml:"IsError,omitempty"`

	// Name of the event. Can be used a C or Go identifier
	Name string `yaml:"Name"`

	// Description of the event
	Description string `yaml:"Description"`

	// Optional fields
	Fields []string `yaml:"Fields,omitempty"`
}
