//go:build generate

// Package testfixtures holds the fixtures behind TestRunFunction.
//
// The request-side fixtures are hand-written: input/ (function Input),
// observed/ (observed composite and composed resources) and desired/ (the
// desired composite handed to the function). The response-side fixtures under
// want/ are generated: they are the function's own output for those requests,
// so after bumping github.com/NVIDIA/aicr regenerate them and review the
// diff — it is exactly what the function will now deploy.
//
//go:generate rm -rf want
//go:generate go test ../ -run ^TestRunFunction$ -count=1 -update
package testfixtures
