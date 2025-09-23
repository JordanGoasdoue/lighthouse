package merge

import (
	"github.com/jenkins-x/lighthouse/pkg/config"
	"github.com/jenkins-x/lighthouse/pkg/config/job"
	"github.com/jenkins-x/lighthouse/pkg/plugins"
	"github.com/jenkins-x/lighthouse/pkg/triggerconfig"
	"github.com/pkg/errors"
)

// ConfigMerge merges the repository configuration into the global configuration
func ConfigMerge(cfg *config.Config, pluginsCfg *plugins.Configuration, repoConfig *triggerconfig.Config, repoOwner string, repoName string) error {
	repoKey := repoOwner + "/" + repoName

	repoConfig.Spec.Presubmits = applyPresubmitDefaults(repoConfig.Spec.DefaultPresubmits, repoConfig.Spec.Presubmits)
	repoConfig.Spec.Postsubmits = applyPostsubmitDefaults(repoConfig.Spec.DefaultPostsubmits, repoConfig.Spec.Postsubmits)
	repoConfig.Spec.Periodics = applyPeriodicDefaults(repoConfig.Spec.DefaultPeriodics, repoConfig.Spec.Periodics)
	repoConfig.Spec.Deployments = applyDeploymentDefaults(repoConfig.Spec.DefaultDeployments, repoConfig.Spec.Deployments)

	if len(repoConfig.Spec.Presubmits) > 0 {
		// lets make a new map to avoid concurrent modifications
		m := map[string][]job.Presubmit{}
		if cfg.Presubmits != nil {
			for k, v := range cfg.Presubmits {
				m[k] = append([]job.Presubmit{}, v...)
			}
		}
		cfg.Presubmits = m

		ps := cfg.Presubmits[repoKey]
		for _, p := range repoConfig.Spec.Presubmits {
			found := false
			for i := range ps {
				pt2 := &ps[i]
				if pt2.Name == p.Name {
					*pt2 = p
					found = true
					break
				}
			}
			if !found {
				ps = append(ps, p)
			}
		}
		cfg.Presubmits[repoKey] = ps
	}
	if len(repoConfig.Spec.Postsubmits) > 0 {
		// lets make a new map to avoid concurrent modifications
		m := map[string][]job.Postsubmit{}
		if cfg.Postsubmits != nil {
			for k, v := range cfg.Postsubmits {
				m[k] = append([]job.Postsubmit{}, v...)
			}
		}
		cfg.Postsubmits = m

		ps := cfg.Postsubmits[repoKey]
		for _, p := range repoConfig.Spec.Postsubmits {
			found := false
			for i := range ps {
				pt2 := &ps[i]
				if pt2.Name == p.Name {
					ps[i] = p
					found = true
				}
			}
			if !found {
				ps = append(ps, p)
			}
		}
		cfg.Postsubmits[repoKey] = ps
	}
	if len(repoConfig.Spec.Periodics) > 0 {
		cfg.Periodics = append(cfg.Periodics, repoConfig.Spec.Periodics...)
	}
	if len(repoConfig.Spec.Deployments) > 0 {
		// lets make a new map to avoid concurrent modifications
		m := map[string][]job.Deployment{}
		if cfg.Deployments != nil {
			for k, v := range cfg.Deployments {
				m[k] = append([]job.Deployment{}, v...)
			}
		}
		cfg.Deployments = m

		ps := cfg.Deployments[repoKey]
		for _, p := range repoConfig.Spec.Deployments {
			found := false
			for i := range ps {
				pt2 := &ps[i]
				if pt2.Name == p.Name {
					ps[i] = p
					found = true
				}
			}
			if !found {
				ps = append(ps, p)
			}
		}
		cfg.Deployments[repoKey] = ps
	}

	// lets make sure we've got a trigger added
	idx := len(pluginsCfg.Triggers) - 1
	if idx < 0 {
		idx = 0
		pluginsCfg.Triggers = append(pluginsCfg.Triggers, plugins.Trigger{})
	}
	if StringArrayIndex(pluginsCfg.Triggers[idx].Repos, repoKey) < 0 {
		pluginsCfg.Triggers[idx].Repos = append(pluginsCfg.Triggers[idx].Repos, repoKey)
	}

	// lets validate the configuration is valid
	err := pluginsCfg.Validate()
	if err != nil {
		return errors.Wrapf(err, "failed to validate plugins")
	}
	migrateOldConfig(&cfg.JobConfig)
	err = cfg.Init(cfg.ProwConfig)
	if err != nil {
		return errors.Wrapf(err, "failed to initialize config")
	}
	err = cfg.Validate(cfg.ProwConfig)
	if err != nil {
		return errors.Wrapf(err, "failed to validate config")
	}
	return nil
}

// migrateOldConfig lets handle some old incorrect configuration where the trigger and rerun_command values were not setup properly
func migrateOldConfig(cfg *job.Config) {
	for _, ps := range cfg.Presubmits {
		for i := range ps {
			presubmit := &ps[i]
			if presubmit.Trigger == "/test" && presubmit.RerunCommand == "/retest" {
				presubmit.Trigger = "(?m)^/test,?($|\\s.*)"
				presubmit.RerunCommand = "/test"
			} else if presubmit.Trigger == "/lint" && presubmit.RerunCommand == "/relint" {
				presubmit.Trigger = "(?m)^/lint,?($|\\s.*)"
				presubmit.RerunCommand = "/lint"
			}
		}
	}
}

// MergePresubmit merges a default presubmit with an actual presubmit
func MergePresubmit(defaultJob, actualJob job.Presubmit) job.Presubmit {
	merged := defaultJob

	// Base fields
	if actualJob.Name != "" {
		merged.Name = actualJob.Name
	}
	if actualJob.Agent != "" {
		merged.Agent = actualJob.Agent
	}
	if actualJob.Cluster != "" {
		merged.Cluster = actualJob.Cluster
	}
	if actualJob.Namespace != nil && *actualJob.Namespace != "" {
		merged.Namespace = actualJob.Namespace
	}
	if actualJob.Context != "" {
		merged.Context = actualJob.Context
	}
	if actualJob.MaxConcurrency != 0 {
		merged.MaxConcurrency = actualJob.MaxConcurrency
	}
	if len(actualJob.Labels) > 0 {
		merged.Labels = actualJob.Labels
	}
	if len(actualJob.Annotations) > 0 {
		merged.Annotations = actualJob.Annotations
	}

	// Brancher fields
	if len(actualJob.Branches) > 0 {
		merged.Branches = actualJob.Branches
	}
	if len(actualJob.SkipBranches) > 0 {
		merged.SkipBranches = actualJob.SkipBranches
	}

	// RegexpChangeMatcher fields
	if actualJob.RunIfChanged != "" {
		merged.RunIfChanged = actualJob.RunIfChanged
	}
	if actualJob.IgnoreChanges != "" {
		merged.IgnoreChanges = actualJob.IgnoreChanges
	}

	// Reporter fields
	if actualJob.Context != "" {
		merged.Context = actualJob.Context
	}
	if actualJob.SkipReport {
		merged.SkipReport = actualJob.SkipReport
	}

	// Presubmit-specific fields
	if actualJob.AlwaysRun {
		merged.AlwaysRun = actualJob.AlwaysRun
	}
	if actualJob.RequireRun {
		merged.RequireRun = actualJob.RequireRun
	}
	if actualJob.Optional {
		merged.Optional = actualJob.Optional
	}
	if actualJob.Trigger != "" {
		merged.Trigger = actualJob.Trigger
	}
	if actualJob.RerunCommand != "" {
		merged.RerunCommand = actualJob.RerunCommand
	}
	if actualJob.JenkinsSpec != nil {
		merged.JenkinsSpec = actualJob.JenkinsSpec
	}

	return merged
}

// MergePostsubmit merges a default postsubmit with an actual postsubmit
func MergePostsubmit(defaultJob, actualJob job.Postsubmit) job.Postsubmit {
	merged := defaultJob

	// Base fields
	if actualJob.Name != "" {
		merged.Name = actualJob.Name
	}
	if actualJob.Agent != "" {
		merged.Agent = actualJob.Agent
	}
	if actualJob.Cluster != "" {
		merged.Cluster = actualJob.Cluster
	}
	if actualJob.Namespace != nil && *actualJob.Namespace != "" {
		merged.Namespace = actualJob.Namespace
	}
	if actualJob.Context != "" {
		merged.Context = actualJob.Context
	}
	if actualJob.MaxConcurrency != 0 {
		merged.MaxConcurrency = actualJob.MaxConcurrency
	}
	if len(actualJob.Labels) > 0 {
		merged.Labels = actualJob.Labels
	}
	if len(actualJob.Annotations) > 0 {
		merged.Annotations = actualJob.Annotations
	}

	// RegexpChangeMatcher fields
	if actualJob.RunIfChanged != "" {
		merged.RunIfChanged = actualJob.RunIfChanged
	}
	if actualJob.IgnoreChanges != "" {
		merged.IgnoreChanges = actualJob.IgnoreChanges
	}

	// Brancher fields
	if len(actualJob.Branches) > 0 {
		merged.Branches = actualJob.Branches
	}
	if len(actualJob.SkipBranches) > 0 {
		merged.SkipBranches = actualJob.SkipBranches
	}

	// Reporter fields
	if actualJob.SkipReport {
		merged.SkipReport = actualJob.SkipReport
	}

	// Postsubmit-specific fields
	if actualJob.JenkinsSpec != nil {
		merged.JenkinsSpec = actualJob.JenkinsSpec
	}

	return merged
}

// MergePeriodic merges a default periodic with an actual periodic
func MergePeriodic(defaultJob, actualJob job.Periodic) job.Periodic {
	merged := defaultJob

	// Base fields
	if actualJob.Name != "" {
		merged.Name = actualJob.Name
	}
	if actualJob.Agent != "" {
		merged.Agent = actualJob.Agent
	}
	if actualJob.Cluster != "" {
		merged.Cluster = actualJob.Cluster
	}
	if actualJob.Namespace != nil && *actualJob.Namespace != "" {
		merged.Namespace = actualJob.Namespace
	}
	if actualJob.Context != "" {
		merged.Context = actualJob.Context
	}
	if actualJob.MaxConcurrency != 0 {
		merged.MaxConcurrency = actualJob.MaxConcurrency
	}
	if len(actualJob.Labels) > 0 {
		merged.Labels = actualJob.Labels
	}
	if len(actualJob.Annotations) > 0 {
		merged.Annotations = actualJob.Annotations
	}

	// Reporter fields
	if actualJob.SkipReport {
		merged.SkipReport = actualJob.SkipReport
	}

	// Periodic-specific fields
	if actualJob.Cron != "" {
		merged.Cron = actualJob.Cron
	}
	if actualJob.Branch != "" {
		merged.Branch = actualJob.Branch
	}

	return merged
}

// MergeDeployment merges a default deployment with an actual deployment
func MergeDeployment(defaultJob, actualJob job.Deployment) job.Deployment {
	merged := defaultJob

	// Base fields
	if actualJob.Name != "" {
		merged.Name = actualJob.Name
	}
	if actualJob.Agent != "" {
		merged.Agent = actualJob.Agent
	}
	if actualJob.Cluster != "" {
		merged.Cluster = actualJob.Cluster
	}
	if actualJob.Namespace != nil && *actualJob.Namespace != "" {
		merged.Namespace = actualJob.Namespace
	}
	if actualJob.Context != "" {
		merged.Context = actualJob.Context
	}
	if actualJob.MaxConcurrency != 0 {
		merged.MaxConcurrency = actualJob.MaxConcurrency
	}
	if len(actualJob.Labels) > 0 {
		merged.Labels = actualJob.Labels
	}
	if len(actualJob.Annotations) > 0 {
		merged.Annotations = actualJob.Annotations
	}

	// Reporter fields
	if actualJob.SkipReport {
		merged.SkipReport = actualJob.SkipReport
	}

	// Deployment-specific fields
	if actualJob.State != "" {
		merged.State = actualJob.State
	}
	if actualJob.Environment != "" {
		merged.Environment = actualJob.Environment
	}

	return merged
}

// applyPresubmitDefaults merges default presubmits with actual presubmits
func applyPresubmitDefaults(defaults []job.Presubmit, jobs []job.Presubmit) []job.Presubmit {
	if len(defaults) == 0 {
		return jobs
	}

	defaultsMap := make(map[string]job.Presubmit)
	for _, d := range defaults {
		defaultsMap[d.Name] = d
	}

	var result []job.Presubmit
	for _, job := range jobs {
		if defaultJob, exists := defaultsMap[job.Name]; exists {
			merged := MergePresubmit(defaultJob, job)
			result = append(result, merged)
		} else {
			result = append(result, job)
		}
	}

	return result
}

// applyPostsubmitDefaults merges default postsubmits with actual postsubmits
func applyPostsubmitDefaults(defaults []job.Postsubmit, jobs []job.Postsubmit) []job.Postsubmit {
	if len(defaults) == 0 {
		return jobs
	}

	defaultsMap := make(map[string]job.Postsubmit)
	for _, d := range defaults {
		defaultsMap[d.Name] = d
	}

	var result []job.Postsubmit
	for _, job := range jobs {
		if defaultJob, exists := defaultsMap[job.Name]; exists {
			merged := MergePostsubmit(defaultJob, job)
			result = append(result, merged)
		} else {
			result = append(result, job)
		}
	}

	return result
}

// applyPeriodicDefaults merges default periodics with actual periodics
func applyPeriodicDefaults(defaults []job.Periodic, jobs []job.Periodic) []job.Periodic {
	if len(defaults) == 0 {
		return jobs
	}

	defaultsMap := make(map[string]job.Periodic)
	for _, d := range defaults {
		defaultsMap[d.Name] = d
	}

	var result []job.Periodic
	for _, job := range jobs {
		if defaultJob, exists := defaultsMap[job.Name]; exists {
			merged := MergePeriodic(defaultJob, job)
			result = append(result, merged)
		} else {
			result = append(result, job)
		}
	}

	return result
}

// applyDeploymentDefaults merges default deployments with actual deployments
func applyDeploymentDefaults(defaults []job.Deployment, jobs []job.Deployment) []job.Deployment {
	if len(defaults) == 0 {
		return jobs
	}

	defaultsMap := make(map[string]job.Deployment)
	for _, d := range defaults {
		defaultsMap[d.Name] = d
	}

	var result []job.Deployment
	for _, job := range jobs {
		if defaultJob, exists := defaultsMap[job.Name]; exists {
			merged := MergeDeployment(defaultJob, job)
			result = append(result, merged)
		} else {
			result = append(result, job)
		}
	}

	return result
}
