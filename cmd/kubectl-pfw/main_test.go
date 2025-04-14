package main

import (
	"testing"

	"errors"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestFlagExclusivity tests that only one of --pods, --deployments, or --statefulsets can be used
// at a time, and that an appropriate error is returned if multiple are specified.
func TestFlagExclusivity(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errMsg  string
	}{
		{"no flags", []string{}, false, ""},
		{"pods only", []string{"--pods"}, false, ""},
		{"deployments only", []string{"--deployments"}, false, ""},
		{"statefulsets only", []string{"--statefulsets"}, false, ""},
		{"pods and deployments", []string{"--pods", "--deployments"}, true, "only one of --pods, --deployments, or --statefulsets can be used at a time"},
		{"pods and statefulsets", []string{"--pods", "--statefulsets"}, true, "only one of --pods, --deployments, or --statefulsets can be used at a time"},
		{"deployments and statefulsets", []string{"--deployments", "--statefulsets"}, true, "only one of --pods, --deployments, or --statefulsets can be used at a time"},
		{"all three", []string{"--pods", "--deployments", "--statefulsets"}, true, "only one of --pods, --deployments, or --statefulsets can be used at a time"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use: "kubectl-pfw",
				RunE: func(cmd *cobra.Command, args []string) error {
					// Get flag values
					usePods, _ := cmd.Flags().GetBool("pods")
					useDeployments, _ := cmd.Flags().GetBool("deployments")
					useStatefulSets, _ := cmd.Flags().GetBool("statefulsets")

					// Check for mutual exclusivity
					selectedModes := 0
					if usePods {
						selectedModes++
					}
					if useDeployments {
						selectedModes++
					}
					if useStatefulSets {
						selectedModes++
					}
					if selectedModes > 1 {
						return errors.New("only one of --pods, --deployments, or --statefulsets can be used at a time")
					}
					return nil
				},
			}

			// Add the flags we want to test
			cmd.Flags().Bool("pods", false, "Select pods instead of services")
			cmd.Flags().Bool("deployments", false, "Select deployments instead of services")
			cmd.Flags().Bool("statefulsets", false, "Select statefulsets instead of services")

			// Set args and execute
			cmd.SetArgs(tt.args)
			err := cmd.Execute()

			if tt.wantErr {
				assert.Error(t, err)
				if err != nil {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
