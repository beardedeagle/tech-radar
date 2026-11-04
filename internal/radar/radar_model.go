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
		if lifecycle := canonicalLifecycle(f.Decision.Lifecycle); m.decisionModel(f.Decision) == LifecycleModel && lifecycle != "" {
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
	return "Lifecycle model. Subsequent generation. Included technologies missing a decision reference require review."
}

func modelRingExplanation(model Model) string {
	if model == LifecycleModel {
		return "Adopt is innermost, followed by Experiment, Hold and Retire. Supplied lifecycle decisions determine placement. Needs decision lists unplaced candidates. Missing decision material requires review on subsequent generations."
	}
	return "Adopt reflects established source use or a saved decision. Trial suggests a limited trial. Assess calls for investigation. Caution records concerns. Suggested placements need team review. Lifecycle is recorded separately."
}

// Legacy lifecycle exports also carried recommendation records without provenance.
// Retain their free-text notes and recommendation approvals on read.
func (m Model) decisionModel(d Decision) Model {
	if d.Model != "" {
		return d.Model
	}
	if m == LifecycleModel && canonicalLifecycle(d.Lifecycle) == "" && (d.Lifecycle != "" || d.Ring != "") {
		return RecommendationModel
	}
	return m.effective()
}

func validateSavedDecisions(r Report) error {
	for _, id := range sortedKeys(r.Decisions) {
		d := r.Decisions[id]
		if err := validateModelRules(map[string]Decision{id: d}, r.Model.decisionModel(d)); err != nil {
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
