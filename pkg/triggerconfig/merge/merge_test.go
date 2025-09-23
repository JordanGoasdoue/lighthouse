package merge_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jenkins-x/lighthouse/pkg/config"
	"github.com/jenkins-x/lighthouse/pkg/config/job"
	"github.com/jenkins-x/lighthouse/pkg/plugins"
	"github.com/jenkins-x/lighthouse/pkg/triggerconfig"
	"github.com/jenkins-x/lighthouse/pkg/triggerconfig/merge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func TestMergeTriggerConfig(t *testing.T) {
	testCases := []struct {
		name       string
		cfg        config.Config
		pluginCfg  plugins.Configuration
		repoConfig triggerconfig.Config
	}{
		{
			name: "emptyConfig",
			repoConfig: triggerconfig.Config{
				Spec: triggerconfig.ConfigSpec{
					Presubmits: []job.Presubmit{
						{
							Base: job.Base{
								Name:  "lint",
								Agent: job.TektonPipelineAgent,
							},
							AlwaysRun:    true,
							Optional:     false,
							Trigger:      "(?:/lint|/relint)",
							RerunCommand: "/relint",
							Reporter: job.Reporter{
								Context: "lint",
							},
						},
					},
					Postsubmits: []job.Postsubmit{
						{
							Base: job.Base{
								Name:  "release",
								Agent: job.TektonPipelineAgent,
							},
							Reporter: job.Reporter{
								Context: "release",
							},
						},
					},
				},
			},
		},
	}

	repoOwner := "myorg"
	repoName := "myowner"
	repoKey := repoOwner + "/" + repoName

	for _, tc := range testCases {
		name := tc.name
		err := merge.ConfigMerge(&tc.cfg, &tc.pluginCfg, &tc.repoConfig, repoOwner, repoName)
		require.NoError(t, err, "failed to merge repository config for %s", name)

		assert.Equal(t, len(tc.repoConfig.Spec.Presubmits), len(tc.cfg.Presubmits[repoKey]), "presubmits for %s", name)
		t.Logf("test %s has %d presubmits for repository key %s", name, len(tc.cfg.Presubmits[repoKey]), repoKey)

		assert.Equal(t, len(tc.repoConfig.Spec.Postsubmits), len(tc.cfg.Postsubmits[repoKey]), "postsubmits for %s", name)
		t.Logf("test %s has %d postsubmits for repository key %s", name, len(tc.cfg.Postsubmits[repoKey]), repoKey)
	}
}

func TestMergeTriggerConfigFiles(t *testing.T) {
	sourceData := "test_data"
	fileNames, err := os.ReadDir(sourceData)
	assert.NoError(t, err)

	repoOwner := "myorg"
	repoName := "myowner"

	for _, f := range fileNames {
		if f.IsDir() {
			name := f.Name()
			srcConfigFile := filepath.Join(sourceData, name, "source-config.yaml")
			srcPluginsFile := filepath.Join(sourceData, name, "source-plugins.yaml")
			expectedConfigFile := filepath.Join(sourceData, name, "expected-config.yaml")
			expectedPluginsFile := filepath.Join(sourceData, name, "expected-plugins.yaml")
			repoConfigFile := filepath.Join(sourceData, name, "triggers.yaml")
			require.FileExists(t, srcConfigFile)
			require.FileExists(t, expectedConfigFile)
			require.FileExists(t, expectedPluginsFile)
			require.FileExists(t, repoConfigFile)

			cfg := &config.Config{}
			pluginCfg := &plugins.Configuration{}
			repoConfig := &triggerconfig.Config{}
			LoadYAMLFile(t, srcConfigFile, cfg)
			LoadYAMLFile(t, srcPluginsFile, pluginCfg)
			LoadYAMLFile(t, repoConfigFile, repoConfig)

			err := merge.ConfigMerge(cfg, pluginCfg, repoConfig, repoOwner, repoName)
			require.NoError(t, err, "failed to merge files in dir %s", name)

			resultConfigText := ToYAMLString(t, cfg, name)
			resultPluginsText := ToYAMLString(t, pluginCfg, name)

			expectedConfigText := LoadTrimmedText(t, expectedConfigFile)
			expectedPluginsText := LoadTrimmedText(t, expectedPluginsFile)

			if d := cmp.Diff(strings.TrimSpace(resultConfigText), expectedConfigText); d != "" {
				t.Errorf("Generated config did not match expected: %s", d)
			}
			if d := cmp.Diff(strings.TrimSpace(resultPluginsText), expectedPluginsText); d != "" {
				t.Errorf("Generated plugins did not match expected: %s", d)
			}
			t.Logf("generated for file %s\n%s\n", srcConfigFile, resultConfigText)
			t.Logf("generated for file %s\n%s\n", srcPluginsFile, resultPluginsText)
		}
	}
}

func LoadTrimmedText(t *testing.T, expectedConfigFile string) string {
	expectData, err := os.ReadFile(expectedConfigFile)
	require.NoError(t, err, "failed to load results %s", expectedConfigFile)
	expectedText := strings.TrimSpace(string(expectData))
	return expectedText
}

func ToYAMLString(t *testing.T, cfg interface{}, testName string) string {
	resultData, err := yaml.Marshal(&cfg)
	require.NoError(t, err, "failed to marshal config for %s", testName)
	resultText := string(resultData)
	return resultText
}

// LoadFile loads the given YAML file
func LoadYAMLFile(t *testing.T, fileName string, dest interface{}) {
	require.FileExists(t, fileName)

	data, err := os.ReadFile(fileName)
	require.NoError(t, err, "failed to read file %s", fileName)

	err = yaml.Unmarshal(data, dest)
	require.NoError(t, err, "failed to unmarshal YAML file %s", fileName)
}

func TestMergeTriggerConfigWithDefaults(t *testing.T) {
	testCases := []struct {
		name       string
		cfg        config.Config
		pluginCfg  plugins.Configuration
		repoConfig triggerconfig.Config
		expected   config.Config
	}{
		// ========== BASIC INHERITANCE TESTS ==========
		{
			name: "presubmitWithDefaults",
			// Tests: Basic field inheritance and override behavior for presubmits
			// Default provides: Agent, AlwaysRun, Optional, IgnoreChanges, Context
			// Actual provides: Context (override), Branches (new field)
			// Expected: Inherits defaults except Context which is overridden
			repoConfig: triggerconfig.Config{
				Spec: triggerconfig.ConfigSpec{
					DefaultPresubmits: []job.Presubmit{
						{
							Base: job.Base{
								Name:  "build",
								Agent: job.TektonPipelineAgent,
							},
							AlwaysRun: true,
							Optional:  false,
							RegexpChangeMatcher: job.RegexpChangeMatcher{
								IgnoreChanges: "*.md",
							},
							Reporter: job.Reporter{
								Context: "default-build",
							},
						},
					},
					Presubmits: []job.Presubmit{
						{
							Base: job.Base{
								Name:  "build",
								Agent: job.TektonPipelineAgent,
							},
							Reporter: job.Reporter{
								Context: "actual-build", // Should override default
							},
							Brancher: job.Brancher{
								Branches: []string{"main", "master"}, // Should be added
							},
						},
					},
				},
			},
		},

		{
			name: "postsubmitWithDefaults",
			// Tests: Basic field inheritance and override behavior for postsubmits
			// Default provides: Agent, IgnoreChanges, Context
			// Actual provides: Context (override), Branches (new field)
			// Expected: Inherits Agent and IgnoreChanges, overrides Context, adds Branches
			repoConfig: triggerconfig.Config{
				Spec: triggerconfig.ConfigSpec{
					DefaultPostsubmits: []job.Postsubmit{
						{
							Base: job.Base{
								Name:  "release",
								Agent: job.TektonPipelineAgent,
							},
							RegexpChangeMatcher: job.RegexpChangeMatcher{
								IgnoreChanges: "docs/**",
							},
							Reporter: job.Reporter{
								Context: "default-release",
							},
						},
					},
					Postsubmits: []job.Postsubmit{
						{
							Base: job.Base{
								Name: "release",
							},
							Reporter: job.Reporter{
								Context: "actual-release", // Should override default
							},
							Brancher: job.Brancher{
								Branches: []string{"main"}, // Should be added
							},
						},
					},
				},
			},
		},

		{
			name: "periodicWithDefaults",
			// Tests: Periodic-specific field inheritance (Cron, Branch)
			// Default provides: Agent, Cron
			// Actual provides: Branch (new field)
			// Expected: Inherits Agent and Cron, adds Branch
			repoConfig: triggerconfig.Config{
				Spec: triggerconfig.ConfigSpec{
					DefaultPeriodics: []job.Periodic{
						{
							Base: job.Base{
								Name:  "nightly",
								Agent: job.TektonPipelineAgent,
							},
							Cron: "0 2 * * *",
						},
					},
					Periodics: []job.Periodic{
						{
							Base: job.Base{
								Name: "nightly",
							},
							Branch: "main", // Should be added
						},
					},
				},
			},
		},

		{
			name: "deploymentWithDefaults",
			// Tests: Deployment-specific field inheritance (State, Environment)
			// Default provides: Agent, State
			// Actual provides: Environment (new field)
			// Expected: Inherits Agent and State, adds Environment
			repoConfig: triggerconfig.Config{
				Spec: triggerconfig.ConfigSpec{
					DefaultDeployments: []job.Deployment{
						{
							Base: job.Base{
								Name:  "deploy",
								Agent: job.TektonPipelineAgent,
							},
							State: "success",
						},
					},
					Deployments: []job.Deployment{
						{
							Base: job.Base{
								Name: "deploy",
							},
							Environment: "production", // Should be added
						},
					},
				},
			},
		},

		// ========== NO MATCHING SCENARIOS ==========
		{
			name: "noMatchingDefaults",
			// Tests: Jobs with no matching defaults (by name) remain unchanged
			// Default defines: "different-job" with AlwaysRun=true
			// Actual defines: "actual-job" with Context
			// Expected: "actual-job" remains unchanged (no matching default)
			repoConfig: triggerconfig.Config{
				Spec: triggerconfig.ConfigSpec{
					DefaultPresubmits: []job.Presubmit{
						{
							Base: job.Base{
								Name: "different-job",
							},
							AlwaysRun: true,
						},
					},
					Presubmits: []job.Presubmit{
						{
							Base: job.Base{
								Name:  "actual-job",
								Agent: job.TektonPipelineAgent,
							},
							Reporter: job.Reporter{
								Context: "actual-context",
							},
						},
					},
				},
			},
		},

		{
			name: "emptyDefaults",
			// Tests: When no defaults are defined, jobs pass through unchanged
			// No defaults provided
			// Actual defines: job with Agent, AlwaysRun, Context
			// Expected: Job remains exactly as defined (baseline behavior)
			repoConfig: triggerconfig.Config{
				Spec: triggerconfig.ConfigSpec{
					Presubmits: []job.Presubmit{
						{
							Base: job.Base{
								Name:  "build",
								Agent: job.TektonPipelineAgent,
							},
							AlwaysRun: true,
							Reporter: job.Reporter{
								Context: "build",
							},
						},
					},
				},
			},
		},

		// ========== MULTIPLE JOBS SCENARIOS ==========
		{
			name: "mixedJobMatching",
			// Tests: Multiple jobs with some matching defaults, some not
			// Defaults: "build" (AlwaysRun=true), "test" (Optional=true)
			// Actuals: "build" (matches, Context override), "lint" (no match), "test" (matches, Context override)
			// Expected: "build" inherits+overrides, "lint" unchanged, "test" inherits+overrides
			repoConfig: triggerconfig.Config{
				Spec: triggerconfig.ConfigSpec{
					DefaultPresubmits: []job.Presubmit{
						{
							Base:      job.Base{Name: "build"},
							AlwaysRun: true,
							Reporter:  job.Reporter{Context: "default-build"},
						},
						{
							Base:     job.Base{Name: "test"},
							Optional: true,
							Reporter: job.Reporter{Context: "default-test"},
						},
					},
					Presubmits: []job.Presubmit{
						{
							Base:     job.Base{Name: "build", Agent: job.TektonPipelineAgent},
							Reporter: job.Reporter{Context: "ci-build"}, // Should override default
						},
						{
							Base:     job.Base{Name: "lint", Agent: job.TektonPipelineAgent},
							Reporter: job.Reporter{Context: "ci-lint"}, // No matching default
						},
						{
							Base:     job.Base{Name: "test", Agent: job.TektonPipelineAgent},
							Reporter: job.Reporter{Context: "ci-test"}, // Should override default
						},
					},
				},
			},
		},

		// ========== COMPLEX FIELD TYPES ==========
		{
			name: "complexFieldTypes",
			// Tests: Complex field types (maps, integers) inheritance and override
			// Default provides: Labels map, Annotations map, MaxConcurrency integer
			// Actual provides: Different Labels map (should replace), higher MaxConcurrency (should override)
			// Expected: Labels completely replaced, Annotations inherited, MaxConcurrency overridden
			repoConfig: triggerconfig.Config{
				Spec: triggerconfig.ConfigSpec{
					DefaultPresubmits: []job.Presubmit{
						{
							Base: job.Base{
								Name: "build",
								Labels: map[string]string{
									"env":  "ci",
									"team": "platform",
								},
								Annotations: map[string]string{
									"owner": "devops",
								},
								MaxConcurrency: 5,
							},
						},
					},
					Presubmits: []job.Presubmit{
						{
							Base: job.Base{
								Name: "build",
								Labels: map[string]string{
									"env": "prod", // Should completely replace default labels
								},
								MaxConcurrency: 10, // Should override default
							},
						},
					},
				},
			},
		},

		{
			name: "nilFieldHandling",
			// Tests: Nil/empty field handling - defaults should be inherited when actual has nil/empty
			// Default provides: Labels, Annotations
			// Actual provides: no Labels/Annotations (nil/empty)
			// Expected: Should inherit default Labels and Annotations
			repoConfig: triggerconfig.Config{
				Spec: triggerconfig.ConfigSpec{
					DefaultPresubmits: []job.Presubmit{
						{
							Base: job.Base{
								Name: "build",
								Labels: map[string]string{
									"team": "platform",
								},
								Annotations: map[string]string{
									"owner": "devops",
								},
							},
							AlwaysRun: true,
						},
					},
					Presubmits: []job.Presubmit{
						{
							Base: job.Base{Name: "build"}, // No labels/annotations - should inherit
						},
					},
				},
			},
		},

		// ========== INTEGRATION TEST ==========
		{
			name: "allJobTypesWithDefaults",
			// Tests: All job types (presubmit, postsubmit, periodic, deployment) with defaults at once
			// Ensures the system handles multiple job types simultaneously without conflicts
			// Each job type has its own defaults and actual jobs with different override patterns
			repoConfig: triggerconfig.Config{
				Spec: triggerconfig.ConfigSpec{
					// Presubmit: Default AlwaysRun, Actual Context
					DefaultPresubmits: []job.Presubmit{
						{
							Base:      job.Base{Name: "build"},
							AlwaysRun: true,
							Reporter:  job.Reporter{Context: "default-build"},
						},
					},
					Presubmits: []job.Presubmit{
						{
							Base:     job.Base{Name: "build"},
							Reporter: job.Reporter{Context: "ci-build"},
						},
					},
					// Postsubmit: Default IgnoreChanges, Actual Context
					DefaultPostsubmits: []job.Postsubmit{
						{
							Base: job.Base{Name: "release"},
							RegexpChangeMatcher: job.RegexpChangeMatcher{
								IgnoreChanges: "*.md",
							},
							Reporter: job.Reporter{Context: "default-release"},
						},
					},
					Postsubmits: []job.Postsubmit{
						{
							Base:     job.Base{Name: "release"},
							Reporter: job.Reporter{Context: "ci-release"},
						},
					},
					// Periodic: Default Cron, Actual Branch
					DefaultPeriodics: []job.Periodic{
						{
							Base: job.Base{Name: "nightly"},
							Cron: "0 2 * * *",
						},
					},
					Periodics: []job.Periodic{
						{
							Base:   job.Base{Name: "nightly"},
							Branch: "main",
						},
					},
					// Deployment: Default State, Actual Environment
					DefaultDeployments: []job.Deployment{
						{
							Base:  job.Base{Name: "deploy"},
							State: "success",
						},
					},
					Deployments: []job.Deployment{
						{
							Base:        job.Base{Name: "deploy"},
							Environment: "prod",
						},
					},
				},
			},
		},

		// ========== EDGE CASES ==========
		{
			name: "specialCharactersInName",
			// Tests: Edge case with special characters in job names (valid ones)
			// Job names with dots, dashes, underscores should work
			// Tests robustness of name-based matching with special characters
			repoConfig: triggerconfig.Config{
				Spec: triggerconfig.ConfigSpec{
					DefaultPresubmits: []job.Presubmit{
						{
							Base:      job.Base{Name: "test-job_with.special-chars"}, // Valid special chars
							AlwaysRun: true,
							Reporter:  job.Reporter{Context: "default-context"},
						},
					},
					Presubmits: []job.Presubmit{
						{
							Base:     job.Base{Name: "test-job_with.special-chars"}, // Should match exactly
							Reporter: job.Reporter{Context: "actual-context"},
						},
					},
				},
			},
		},
	}

	repoOwner := "myorg"
	repoName := "myrepo"
	repoKey := repoOwner + "/" + repoName

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := merge.ConfigMerge(&tc.cfg, &tc.pluginCfg, &tc.repoConfig, repoOwner, repoName)
			require.NoError(t, err, "failed to merge repository config for %s", tc.name)

			// ========== PRESUBMITS VALIDATION ==========
			if len(tc.repoConfig.Spec.Presubmits) > 0 || len(tc.repoConfig.Spec.DefaultPresubmits) > 0 {
				assert.Equal(t, len(tc.repoConfig.Spec.Presubmits), len(tc.cfg.Presubmits[repoKey]),
					"presubmits count for %s", tc.name)

				switch tc.name {
				case "presubmitWithDefaults":
					presubmit := tc.cfg.Presubmits[repoKey][0]
					assert.Equal(t, "build", presubmit.Name)
					assert.Equal(t, "actual-build", presubmit.Context, "actual context should override default")
					assert.Equal(t, "*.md", presubmit.IgnoreChanges, "should inherit default ignore_changes")
					assert.True(t, presubmit.AlwaysRun, "should inherit default always_run")
					assert.Equal(t, []string{"main", "master"}, presubmit.Branches, "should use actual branches")

				case "mixedJobMatching":
					// Should have 3 jobs: build (merged), lint (unchanged), test (merged)
					assert.Len(t, tc.cfg.Presubmits[repoKey], 3, "should have 3 presubmits")

					// Find each job and verify
					jobs := tc.cfg.Presubmits[repoKey]
					buildJob := findJobByName(jobs, "build")
					lintJob := findJobByName(jobs, "lint")
					testJob := findJobByName(jobs, "test")

					assert.NotNil(t, buildJob, "should find build job")
					assert.NotNil(t, lintJob, "should find lint job")
					assert.NotNil(t, testJob, "should find test job")

					assert.True(t, buildJob.AlwaysRun, "build should inherit AlwaysRun from default")
					assert.Equal(t, "ci-build", buildJob.Context, "build should override context")
					assert.Equal(t, "ci-lint", lintJob.Context, "lint should keep its context (no default)")
					assert.True(t, testJob.Optional, "test should inherit Optional from default")
					assert.Equal(t, "ci-test", testJob.Context, "test should override context")

				case "complexFieldTypes":
					presubmit := tc.cfg.Presubmits[repoKey][0]
					assert.Equal(t, map[string]string{"env": "prod"}, presubmit.Labels,
						"labels should be completely replaced")
					assert.Equal(t, map[string]string{"owner": "devops"}, presubmit.Annotations,
						"annotations should be inherited from default")
					assert.Equal(t, 10, presubmit.MaxConcurrency, "maxConcurrency should be overridden")

				case "nilFieldHandling":
					presubmit := tc.cfg.Presubmits[repoKey][0]
					assert.Equal(t, map[string]string{"team": "platform"}, presubmit.Labels,
						"should inherit default labels when actual has none")
					assert.Equal(t, map[string]string{"owner": "devops"}, presubmit.Annotations,
						"should inherit default annotations when actual has none")
					assert.True(t, presubmit.AlwaysRun, "should inherit default AlwaysRun")
				}
			}

			// ========== POSTSUBMITS VALIDATION ==========
			if len(tc.repoConfig.Spec.Postsubmits) > 0 || len(tc.repoConfig.Spec.DefaultPostsubmits) > 0 {
				assert.Equal(t, len(tc.repoConfig.Spec.Postsubmits), len(tc.cfg.Postsubmits[repoKey]),
					"postsubmits count for %s", tc.name)

				if tc.name == "postsubmitWithDefaults" {
					postsubmit := tc.cfg.Postsubmits[repoKey][0]
					assert.Equal(t, "release", postsubmit.Name)
					assert.Equal(t, "actual-release", postsubmit.Context, "actual context should override default")
					assert.Equal(t, "docs/**", postsubmit.IgnoreChanges, "should inherit default ignore_changes")
					assert.Equal(t, []string{"main"}, postsubmit.Branches, "should use actual branches")
				}
			}

			// ========== PERIODICS VALIDATION ==========
			if len(tc.repoConfig.Spec.Periodics) > 0 || len(tc.repoConfig.Spec.DefaultPeriodics) > 0 {
				assert.Equal(t, len(tc.repoConfig.Spec.Periodics), len(tc.cfg.Periodics),
					"periodics count for %s", tc.name)

				if tc.name == "periodicWithDefaults" {
					var periodic *job.Periodic
					for i := range tc.cfg.Periodics {
						if tc.cfg.Periodics[i].Name == "nightly" {
							periodic = &tc.cfg.Periodics[i]
							break
						}
					}
					require.NotNil(t, periodic, "should find nightly periodic job")
					assert.Equal(t, "0 2 * * *", periodic.Cron, "should inherit default cron")
					assert.Equal(t, "main", periodic.Branch, "should use actual branch")
				}
			}

			// ========== DEPLOYMENTS VALIDATION ==========
			if len(tc.repoConfig.Spec.Deployments) > 0 || len(tc.repoConfig.Spec.DefaultDeployments) > 0 {
				assert.Equal(t, len(tc.repoConfig.Spec.Deployments), len(tc.cfg.Deployments[repoKey]),
					"deployments count for %s", tc.name)

				if tc.name == "deploymentWithDefaults" {
					deployment := tc.cfg.Deployments[repoKey][0]
					assert.Equal(t, "deploy", deployment.Name)
					assert.Equal(t, "success", deployment.State, "should inherit default state")
					assert.Equal(t, "production", deployment.Environment, "should use actual environment")
				}
			}

			// ========== INTEGRATION TEST VALIDATION ==========
			if tc.name == "allJobTypesWithDefaults" {
				// Validate all job types are processed correctly
				presubmit := tc.cfg.Presubmits[repoKey][0]
				assert.True(t, presubmit.AlwaysRun, "presubmit should inherit default AlwaysRun")
				assert.Equal(t, "ci-build", presubmit.Context, "presubmit should override context")

				postsubmit := tc.cfg.Postsubmits[repoKey][0]
				assert.Equal(t, "*.md", postsubmit.IgnoreChanges, "postsubmit should inherit default IgnoreChanges")
				assert.Equal(t, "ci-release", postsubmit.Context, "postsubmit should override context")

				var periodic *job.Periodic
				for i := range tc.cfg.Periodics {
					if tc.cfg.Periodics[i].Name == "nightly" {
						periodic = &tc.cfg.Periodics[i]
						break
					}
				}
				require.NotNil(t, periodic, "should find periodic job")
				assert.Equal(t, "0 2 * * *", periodic.Cron, "periodic should inherit default cron")
				assert.Equal(t, "main", periodic.Branch, "periodic should use actual branch")

				deployment := tc.cfg.Deployments[repoKey][0]
				assert.Equal(t, "success", deployment.State, "deployment should inherit default state")
				assert.Equal(t, "prod", deployment.Environment, "deployment should use actual environment")
			}

			t.Logf("test %s completed successfully", tc.name)
		})
	}
}

// Helper function to find a job by name in a slice
func findJobByName(jobs []job.Presubmit, name string) *job.Presubmit {
	for i := range jobs {
		if jobs[i].Name == name {
			return &jobs[i]
		}
	}
	return nil
}
