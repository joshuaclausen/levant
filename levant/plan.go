package levant

import (
	"fmt"

	nomad "github.com/hashicorp/nomad/api"
	nomadStructs "github.com/hashicorp/nomad/nomad/structs"
	"github.com/rs/zerolog/log"
)

var (
	diffTypeAdded  = string(nomadStructs.DiffTypeAdded)
	diffTypeEdited = string(nomadStructs.DiffTypeEdited)
	diffTypeNone   = string(nomadStructs.DiffTypeNone)
)

// plan is the entry point into running the Levant plan function which logs all
// changes anticipated by Nomad of the upcoming job registration. If there are
// no planned changes here, return false to indicate we should stop the process.
func (l *levantDeployment) plan() bool {

	log.Debug().Msg("levant/plan: triggering Nomad plan")

	// Run a plan using the rendered job.
	resp, _, err := l.nomad.Jobs().Plan(l.config.Job, true, nil)
	if err != nil {
		log.Error().Err(err).Msg("levant/plan: unable to run a job plan")
		return false
	}

	switch resp.Diff.Type {

	// If the job is new, then don't print the entire diff but just log that it
	// is a new registration.
	case diffTypeAdded:
		log.Info().Msg("levant/plan: job is a new addition to the cluster")
		return true

	// If there are no changes, then log an error so the user can see this and
	// exit the deployment.
	case diffTypeNone:
		log.Error().Msg("levant/plan: no changes detected for job")
		return false

	// If there are changes, run the planDiff function which is responsible for
	// iterating through the plan and logging all the planned changes.
	case diffTypeEdited:
		planDiff(resp.Diff)
	}

	return true
}

func planDiff(plan *nomad.JobDiff) {

	// Iterate through each TaskGroup.
	for _, tg := range plan.TaskGroups {
		if tg.Type != diffTypeEdited {
			continue
		}
		for _, tgo := range tg.Objects {
			recurseObjDiff(tg.Name, "", tgo)
		}

		// Iterate through each Task.
		for _, t := range tg.Tasks {
			if t.Type != diffTypeEdited {
				continue
			}
			if len(t.Objects) == 0 {
				return
			}
			for _, o := range t.Objects {
				recurseObjDiff(tg.Name, t.Name, o)
			}
		}
	}
}

func recurseObjDiff(g, t string, objDiff *nomad.ObjectDiff) {

	// If we have reached the end of the object tree, and have an edited type
	// with field information then we can interate on the fields to find those
	// which have changed.
	if len(objDiff.Objects) == 0 && len(objDiff.Fields) > 0 && objDiff.Type == diffTypeEdited {
		for _, f := range objDiff.Fields {
			if f.Type != diffTypeEdited {
				continue
			}
			logDiffObj(g, t, objDiff.Name, f.Name, f.Old, f.New)
			continue
		}

	} else {
		// Continue to interate through the object diff objects until such time
		// the above is triggered.
		for _, o := range objDiff.Objects {
			recurseObjDiff(g, t, o)
		}
	}
}

// logDiffObj is a helper function so Levant can log the most accurate and
// useful plan output messages.
func logDiffObj(g, t, objName, fName, fOld, fNew string) {

	var lStart, l string

	// We will always have at least this information to log.
	lEnd := fmt.Sprintf("plan indicates change of %s:%s from %s to %s",
		objName, fName, fOld, fNew)

	// If we have been passed a group name, use this to start the log line.
	if g != "" {
		lStart = fmt.Sprintf("group %s ", g)
	}

	// If we have been passed a task name, append this to the group name.
	if t != "" {
		lStart = lStart + fmt.Sprintf("and task %s ", t)
	}

	// Build the final log message.
	if lStart != "" {
		l = lStart + lEnd
	} else {
		l = lEnd
	}

	log.Info().Msgf("levant/plan: %s", l)
}
