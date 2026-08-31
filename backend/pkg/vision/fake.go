package vision

import "context"

// Fake is a scripted Client for tests. A zero-value Fake succeeds every call
// with an empty result; set the *Result or *Err fields to control behavior.
// ClarifyResults, if set, are consumed one per Clarify call, in order — so a
// multi-round clarification test can script each round's response; once
// exhausted, ClarifyResult (singular) or a zero-value result is used.
type Fake struct {
	RecognizeResult       *RecognizeResult
	RecognizeErr          error
	EstimateWeightsResult *WeightEstimateResult
	EstimateWeightsErr    error
	ClarifyResult         *RecognizeResult
	ClarifyResults        []*RecognizeResult
	ClarifyErr            error
	SelectResult          *SelectResult
	SelectErr             error
	TranslateResult       string
	TranslateErr          error
	DescribeResult        *RecognizeResult
	DescribeErr           error

	// The *Calls fields record each operation's arguments
	// arguments, in order, so a test can assert on what was actually sent
	// (e.g. that a re-analysis used the same photo, that a clarify round
	// carried no image content, or which candidates were offered).
	RecognizeCalls       []RecognizeCall
	EstimateWeightsCalls []EstimateWeightsCall
	ClarifyCalls         []ClarifyCall
	SelectCalls          [][]ItemCandidates
	TranslateCalls       []string
	DescribeCalls        []DescribeCall
}

// DescribeCall records one Describe invocation.
type DescribeCall struct {
	Description     string
	DisplayLanguage string
}

type EstimateWeightsCall struct {
	Image      []byte
	MimeType   string
	Components []WeightEstimateInput
}

// RecognizeCall records one Recognize invocation.
type RecognizeCall struct {
	Image           []byte
	MimeType        string
	Hint            string
	DisplayLanguage string
}

func (f *Fake) EstimateWeights(_ context.Context, image []byte, mimeType string, components []WeightEstimateInput) (*WeightEstimateResult, error) {
	f.EstimateWeightsCalls = append(f.EstimateWeightsCalls, EstimateWeightsCall{Image: image, MimeType: mimeType, Components: components})
	if f.EstimateWeightsErr != nil {
		return nil, f.EstimateWeightsErr
	}
	if f.EstimateWeightsResult != nil {
		return f.EstimateWeightsResult, nil
	}
	return &WeightEstimateResult{}, nil
}

// ClarifyCall records one Clarify invocation.
type ClarifyCall struct {
	PriorItems      []Item
	History         []ClarifyTurn
	DisplayLanguage string
}

func (f *Fake) Recognize(_ context.Context, image []byte, mimeType, hint, displayLanguage string) (*RecognizeResult, error) {
	f.RecognizeCalls = append(f.RecognizeCalls, RecognizeCall{Image: image, MimeType: mimeType, Hint: hint, DisplayLanguage: displayLanguage})
	if f.RecognizeErr != nil {
		return nil, f.RecognizeErr
	}
	if f.RecognizeResult != nil {
		return f.RecognizeResult, nil
	}
	return &RecognizeResult{}, nil
}

func (f *Fake) Clarify(_ context.Context, priorItems []Item, history []ClarifyTurn, displayLanguage string) (*RecognizeResult, error) {
	f.ClarifyCalls = append(f.ClarifyCalls, ClarifyCall{PriorItems: priorItems, History: history, DisplayLanguage: displayLanguage})
	if f.ClarifyErr != nil {
		return nil, f.ClarifyErr
	}
	if idx := len(f.ClarifyCalls) - 1; idx < len(f.ClarifyResults) {
		return f.ClarifyResults[idx], nil
	}
	if f.ClarifyResult != nil {
		return f.ClarifyResult, nil
	}
	return &RecognizeResult{}, nil
}

func (f *Fake) Select(_ context.Context, itemCandidates []ItemCandidates) (*SelectResult, error) {
	f.SelectCalls = append(f.SelectCalls, itemCandidates)
	if f.SelectErr != nil {
		return nil, f.SelectErr
	}
	if f.SelectResult != nil {
		return f.SelectResult, nil
	}
	return &SelectResult{}, nil
}

func (f *Fake) Translate(_ context.Context, query string) (string, error) {
	f.TranslateCalls = append(f.TranslateCalls, query)
	if f.TranslateErr != nil {
		return "", f.TranslateErr
	}
	return f.TranslateResult, nil
}

func (f *Fake) Describe(_ context.Context, description, displayLanguage string) (*RecognizeResult, error) {
	f.DescribeCalls = append(f.DescribeCalls, DescribeCall{Description: description, DisplayLanguage: displayLanguage})
	if f.DescribeErr != nil {
		return nil, f.DescribeErr
	}
	if f.DescribeResult != nil {
		return f.DescribeResult, nil
	}
	return &RecognizeResult{}, nil
}

var _ Client = (*Fake)(nil)
