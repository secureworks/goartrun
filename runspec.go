package main

// config received from atomic-harness
type RunSpec struct {
   Technique  string
   TestName   string
   TestIndex  int

   AtomicsDir string
   TempDir    string
   ResultsDir string
   Username   string

   Inputs     map[string]string
   //Envs       []string

   Stage      string
}

// TestStatus shared with harness in summary json

type TestStatus int

const (
    StatusUnknown TestStatus = iota
    StatusMiscError             // 1
    StatusAtomicNotFound        // 2
    StatusCriteriaNotFound      // 3
    StatusSkipped               // 4
    StatusInvalidArguments      // 5
    StatusRunnerFailure         // 6
    StatusPreReqFail            // 7
    StatusTestFail              // 8
    StatusTestSuccess           // 9
    StatusTelemetryToolFailure  // 10
    StatusValidateFail          // 11
    StatusValidatePartial       // 12
    StatusValidateSuccess       // 13
)
