package webhook

// ResolveSubjectIDForTest exposes the subject resolution to the external test
// package. It is compiled only under test.
var ResolveSubjectIDForTest = resolveSubjectID
