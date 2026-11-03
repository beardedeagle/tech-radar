package radar

import (
	"fmt"
	"strings"
)

// Model identifies the meaning of the rings. An omitted model is recommendation
// for compatibility with existing reports and policies.
type Model string

const (
	// RecommendationModel places technology by recommended commitment.
	RecommendationModel Model = "recommendation"
	// LifecycleModel places technology only by supplied lifecycle decisions.
	LifecycleModel Model = "lifecycle"
)

func validateModel(model Model) error {
	if model != "" && model != RecommendationModel && model != LifecycleModel {
		return fmt.Errorf("unsupported model %q (want recommendation or lifecycle)", model)
	}
	return nil
}

func (m Model) effective() Model {
	if m == "" {
		return RecommendationModel
	}
	return m
}

func selectModel(requested, policy Model, previous *Report) (Model, error) {
	for _, model := range []Model{requested, policy} {
		if err := validateModel(model); err != nil {
			return "", err
		}
	}
	if requested != "" && policy != "" && requested != policy {
		return "", fmt.Errorf("--model %q conflicts with policy model %q", requested, policy)
	}
	if requested != "" {
		return requested, nil
	}
	if policy != "" {
		return policy, nil
	}
	if previous != nil {
		if err := validateModel(previous.Model); err != nil {
			return "", err
		}
		return previous.Model.effective(), nil
	}
	return RecommendationModel, nil
}

func (m Model) rings() []string {
	if m == LifecycleModel {
		return []string{"Adopt", "Experiment", "Hold", "Retire"}
	}
	return Rings
}

func canonicalLifecycle(value string) string {
	for _, lifecycle := range LifecycleModel.rings() {
		if strings.EqualFold(value, lifecycle) {
			return lifecycle
		}
	}
	return ""
}

func (m Model) pending() string {
	if m == LifecycleModel {
		return "Needs decision"
	}
	return "Needs review"
}

func (m Model) ring(f Finding) string {
	if m == LifecycleModel {
		if lifecycle := canonicalLifecycle(f.Decision.Lifecycle); lifecycle != "" {
			return lifecycle
		}
		return m.pending()
	}
	return ring(f)
}

const lifecycleMethod = "Lifecycle method adapted from Andrew Harmel-Law's [Scaling the Practice of Architecture, Conversationally](https://martinfowler.com/articles/scaling-architecture-conversationally.html)."

func modelReviewNote(r Report) string {
	if r.Model != LifecycleModel {
		return "Recommendation model."
	}
	if r.Baseline {
		return "Lifecycle model. Initial baseline. Review gaps for missing decision material begin with the first use of --previous."
	}
	return "Lifecycle model. Subsequent generation. Missing decision material requires review for every included technology."
}

func modelRingExplanation(model Model) string {
	if model == LifecycleModel {
		return "Adopt is innermost, followed by Experiment, Hold and Retire. Supplied lifecycle decisions determine placement. Needs decision lists unplaced candidates. Missing decision material requires review on subsequent generations."
	}
	return "Adopt reflects established source use or a saved decision. Trial suggests a limited trial. Assess calls for investigation. Caution records concerns. Suggested placements need team review. Lifecycle is recorded separately."
}

// Existing recommendation decisions remain historical records after a switch to
// lifecycle. Only canonical lifecycle values can place them in that model.
func validateSavedDecisions(r Report) error {
	for _, id := range sortedKeys(r.Decisions) {
		model := r.Model
		d := r.Decisions[id]
		if model == LifecycleModel && d.Ring != "" && canonicalLifecycle(d.Lifecycle) == "" {
			model = RecommendationModel
		}
		if err := validateModelRules(map[string]Decision{id: d}, model); err != nil {
			return err
		}
	}
	return nil
}

func groupableDecisions(model Model, parent, member Decision) bool {
	if model == LifecycleModel {
		return parent == member
	}
	return parent == (Decision{}) && member == (Decision{})
}
