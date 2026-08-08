package vision

import "context"

// Fake is a scripted Client for tests. A zero-value Fake succeeds both calls
// with an empty result; set the *Result or *Err fields to control behavior.
type Fake struct {
	RecognizeResult *RecognizeResult
	RecognizeErr    error
	SelectResult    *SelectResult
	SelectErr       error

	// RecognizeCalls and SelectCalls record each call's arguments, in order,
	// so a test can assert on what was actually sent (e.g. that a re-analysis
	// used the same photo, or which candidates were offered).
	RecognizeCalls []RecognizeCall
	SelectCalls    [][]ItemCandidates
}

// RecognizeCall records one Recognize invocation.
type RecognizeCall struct {
	Image    []byte
	MimeType string
}

func (f *Fake) Recognize(_ context.Context, image []byte, mimeType string) (*RecognizeResult, error) {
	f.RecognizeCalls = append(f.RecognizeCalls, RecognizeCall{Image: image, MimeType: mimeType})
	if f.RecognizeErr != nil {
		return nil, f.RecognizeErr
	}
	if f.RecognizeResult != nil {
		return f.RecognizeResult, nil
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

var _ Client = (*Fake)(nil)
