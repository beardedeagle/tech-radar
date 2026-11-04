package radar

import "fmt"

// View selects presentation independently of the report's decision model.
type View string

const (
	// RadarView shows quadrant and ring placement with a supporting register.
	RadarView View = "radar"
	// TableView shows decisions grouped by quadrant and technology family.
	TableView View = "table"
)

func validateView(view View) error {
	if view != "" && view != RadarView && view != TableView {
		return fmt.Errorf("unsupported view %q (want radar or table)", view)
	}
	return nil
}

func (r Report) selectedView() View {
	if r.View != "" {
		return r.View
	}
	if r.Model == LifecycleModel {
		return TableView
	}
	return RadarView
}

func selectView(requested View, model Model, previous *Report) (View, error) {
	if err := validateView(requested); err != nil {
		return "", err
	}
	if previous != nil {
		if err := validateView(previous.View); err != nil {
			return "", err
		}
		if requested == "" && previous.Model.effective() == model.effective() {
			return previous.selectedView(), nil
		}
	}
	return (Report{Model: model, View: requested}).selectedView(), nil
}
