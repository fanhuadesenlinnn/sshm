package deploy

// Condition matches a remote exit code using rc_in / rc_not_in, shared by
// failed_when and changed_when.
type Condition struct {
	RCIn    []int `yaml:"rc_in,omitempty" json:"rc_in,omitempty"`
	RCNotIn []int `yaml:"rc_not_in,omitempty" json:"rc_not_in,omitempty"`
}

// Matches reports whether rc satisfies the condition.
func (c Condition) Matches(rc int) bool {
	for _, value := range c.RCIn {
		if rc == value {
			return true
		}
	}
	if len(c.RCNotIn) > 0 {
		for _, value := range c.RCNotIn {
			if rc == value {
				return false
			}
		}
		return true
	}
	return false
}
