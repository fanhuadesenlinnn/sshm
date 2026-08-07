package deployv3

import (
	"fmt"
)

// ValidateCatalog validates every play's shape and tasks without requiring
// target resolution, so validate works before any hosts exist.
func ValidateCatalog(catalog *Catalog) error {
	for _, play := range catalog.Plays {
		if err := validatePlayShape(play); err != nil {
			return fmt.Errorf("%s: play %q: %w", play.Source, play.Name, err)
		}
		tasks, err := expandIncludes(play.Tasks, play.BaseDir, nil, 0)
		if err != nil {
			return err
		}
		vars, err := resolveVars(play, catalog, Overrides{})
		if err != nil {
			return err
		}
		vars = withFactPlaceholders(vars)
		for index, task := range tasks {
			if err := validateTask(task, vars); err != nil {
				return fmt.Errorf("%s: play %q task %d (%s): %w", play.Source, play.Name, index+1, task.DisplayName(index), err)
			}
		}
	}
	return nil
}
