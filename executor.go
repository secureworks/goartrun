package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)


func Execute(test *AtomicTest, runSpec *RunSpec) (*AtomicTest, error, TestStatus) {
	tid := runSpec.Technique
	env := []string{} // TODO

	Println()

	Println("****** EXECUTION PLAN ******")
	Println(" Technique: " + tid)
	Println(" Test:      " + test.Name)

	stage := runSpec.Stage
	if stage != "" {
		Println(" Stage:     " + stage)
	}


	if len(runSpec.Inputs) == 0 {
		Println(" Inputs:    <none>")
	} else {
		Println(" Inputs:    ", runSpec.Inputs);
	}
/*
	if env == nil {
		Println(" Env:       <none>")
	} else {
		Println(" Env:       " + strings.Join(env, "\n            "))
	}
*/
	Println(" * Use at your own risk :) *")
	Println("****************************")

	args, err := checkArgsAndGetDefaults(test, runSpec)
	if err != nil {
		return nil, err, StatusInvalidArguments
	}

	// overwrite with actual args used
	test.ArgsUsed = args

	if err := checkPlatform(test); err != nil {
		return nil, err, StatusInvalidArguments
	}

	//var results string

	stages := []string { "prereq", "test", "cleanup"}
	if "" != stage {
		stages = []string { stage }
	}

	status := StatusUnknown
	for _, stage = range stages {
		switch stage {
		case "cleanup":
			_, err = executeStage(stage, test.Executor.CleanupCommand, test.Executor.Name, test.BaseDir, args , env , tid, test.Name, runSpec)
			if err != nil {
				Println("WARNING. Cleanup command failed", err)
			} else {
				test.IsCleanedUp = true
			}

		case "prereq":
			if len(test.Dependencies) != 0 {
				if IsUnsupportedExecutor(test.Executor.Name) {
					return nil, fmt.Errorf("dependency executor %s is not supported", test.DependencyExecutorName),StatusInvalidArguments
				}

				Printf("\nChecking dependencies...\n")

				for i, dep := range test.Dependencies {
					Printf("  - %s", dep.Description)

					_, err := executeStage(fmt.Sprintf("checkPrereq%d",i), dep.PrereqCommand, test.DependencyExecutorName, test.BaseDir, args , env , tid, test.Name, runSpec)


					if err == nil {
						Printf("   * OK - dependency check succeeded!\n")
						continue
					}

					result, err := executeStage(fmt.Sprintf("getPrereq%d",i), dep.GetPrereqCommand, test.DependencyExecutorName, test.BaseDir, args , env , tid, test.Name, runSpec)

					if err != nil {
						if result == "" {
							result = "no details provided"
						}

						Printf("   * XX - dependency check failed: %s\n", result)

						return nil, fmt.Errorf("not all dependency checks passed"), StatusPreReqFail
					}
				}
			}
		case "test":
			if test.Executor == nil {
				return nil, fmt.Errorf("test has no executor"), StatusInvalidArguments
			}

			if IsUnsupportedExecutor(test.Executor.Name) {
				return nil, fmt.Errorf("executor %s is not supported", test.Executor.Name), StatusInvalidArguments
			}
			test.StartTime = time.Now().UnixNano()

			results, err := executeStage(stage, test.Executor.Command, test.Executor.Name, test.BaseDir, args , env , tid, test.Name, runSpec)

			test.EndTime = time.Now().UnixNano()

			errstr := ""
			if err != nil {
				Println("****** EXECUTOR FAILED ******")
				status = StatusTestFail
				errstr = fmt.Sprint(err)
			} else {
				Println("****** EXECUTOR RESULTS ******")
				status = StatusTestSuccess
			}
			if results != "" {
				Println(results)
				Println("******************************")
			}

			// save state

			for k, v := range test.InputArugments {
				v.ExpectedValue = args[k]
				test.InputArugments[k] = v
			}

			test.Executor.ExecutedCommand = map[string]interface{}{
				"command": test.Executor.Command /* command */,
				"results": results ,
				"err": errstr,
			}

		default:
			Printf("Unknown stage:" + stage)
			return nil, nil, StatusRunnerFailure
		}
	}
	return test, nil, status

}

func IsUnsupportedExecutor(executorName string) bool {
	for _, e := range SupportedExecutors {
		if executorName == e {
			return false
		}
	}
	return true
}

func GetTechnique(tid string, runSpec *RunSpec) (*Atomic, error) {
	if !strings.HasPrefix(tid, "T") {
		tid = "T" + tid
	}

	var body []byte

	if runSpec.AtomicsDir == "" {
		return nil, fmt.Errorf("missing atomic dir")
	}

	// Check to see if test is defined locally first. If not, body will be nil
	// and the test will be loaded below.
	body, _ = os.ReadFile(runSpec.AtomicsDir + "/" + tid + "/" + tid + ".yaml")
	if len(body) == 0 {
		body, _ = os.ReadFile(runSpec.AtomicsDir + "/" + tid + "/" + tid + ".yml")
	}

	if len(body) != 0 {
		var technique Atomic

		if err := yaml.Unmarshal(body, &technique); err != nil {
			return nil, fmt.Errorf("processing Atomic Test YAML file: %w", err)
		}

		technique.BaseDir = runSpec.AtomicsDir
		return &technique, nil
	}

	return nil, fmt.Errorf("missing atomic", tid)
}


func getTest(tid, name string, index int, runSpec *RunSpec) (*AtomicTest, error) {
	Printf("\nGetting Atomic Tests technique %s from %s\n", tid, runSpec.AtomicsDir)

	technique, err := GetTechnique(tid, runSpec)
	if err != nil {
		return nil, fmt.Errorf("getting Atomic Tests technique: %w", err)
	}

	Printf("  - technique has %d tests\n", len(technique.AtomicTests))

	var test *AtomicTest

	if index >= 0 && index < len(technique.AtomicTests) {
		test = &technique.AtomicTests[index]
	} else {
		for _, t := range technique.AtomicTests {
			if t.Name == name {
				test = &t
				break
			}
		}
	}

	if test == nil {
		return nil, fmt.Errorf("could not find test %s/%s", tid, name)
	}

	test.BaseDir = technique.BaseDir
	test.TempDir = runSpec.TempDir

	Printf("  - found test named %s\n", test.Name)

	return test, nil
}

func checkArgsAndGetDefaults(test *AtomicTest, runSpec *RunSpec) (map[string]string, error) {
	var (
		updated = make(map[string]string)
	)

	if len(test.InputArugments) == 0 {
		return updated, nil
	}

	keys := []string{}
	for k := range runSpec.Inputs {
		keys = append(keys, k)
	}

	Println("\nChecking arguments...")

	if len(keys) > 0 {
		Println("  - supplied in config/flags: " + strings.Join(keys, ", "))
	}

	for k, v := range test.InputArugments {
		Println("  - checking for argument " + k)

		val, ok := runSpec.Inputs[k] //args[k]

		if ok {
			Println("   * OK - found argument in supplied args")
		} else {
			Println("   * XX - not found, trying default arg")

			val = v.Default

			if val == "" {
				return nil, fmt.Errorf("argument [%s] is required but not set and has no default", k)
			} else {
				Println("   * OK - found argument in defaults")
			}
		}

		updated[k] = val
	}

	return updated, nil
}

func checkPlatform(test *AtomicTest) error {
	var platform string

	switch runtime.GOOS {
	case "linux", "freebsd", "netbsd", "openbsd", "solaris":
		platform = "linux"
	case "darwin":
		platform = "macos"
	case "windows":
		platform = "windows"
	}

	if platform == "" {
		return fmt.Errorf("unable to detect our platform")
	}

	Printf("\nChecking platform vs our platform (%s)...\n", platform)

	var found bool

	for _, p := range test.SupportedPlatforms {
		if p == platform {
			found = true
			break
		}
	}

	if found {
		Println("  - OK - our platform is supported!")
	} else {
		return fmt.Errorf("unable to run test that supports platforms %v because we are on %s", test.SupportedPlatforms, platform)
	}

	return nil
}

func executeStage(stage, cmds, executorName, base string, args map[string]string, env []string, technique, testName string, runSpec *RunSpec) (string,error) {
	quiet := true

	if stage == "test" {
		quiet = false
	}

	if cmds == "" {
		Println("Test does not have " + stage + " stage defined")
		return "", nil
	}

	command, err := interpolateWithArgs(cmds, base, args, quiet)
	if err != nil {
		Println("    * FAIL - " + stage + " failed", err)
		return "", err
	}

	if 0 == len(executorName) {
		executorName = "sh"
		fmt.Println("no",stage,"executor specified. using sh")
	}

	var results string
	switch executorName {
	case "bash":
		results, err = executeShell("bash",command, env, stage, technique, testName, runSpec)
	case "sh":
		results, err = executeShell("sh",command, env, stage, technique, testName, runSpec)
	default:
		err = fmt.Errorf("unknown executor: " + executorName)
	}

	if err != nil {
		Printf("   * FAIL - " + stage + " failed!\n", err)
		return results, err
	}
	Printf("   * OK - " + stage + " succeeded!\n")
	return results, nil
}

func interpolateWithArgs(interpolatee, base string, args map[string]string, quiet bool) (string, error) {
	interpolated := strings.TrimSpace(interpolatee)

	// replace folder path if present in script

	interpolated = strings.ReplaceAll(interpolated, "$PathToAtomicsFolder", base)
	interpolated = strings.ReplaceAll(interpolated, "PathToAtomicsFolder", base)

	if len(args) == 0 {
		return interpolated, nil
	}

	// is this Quiet business doing anything anymore?
	prevQuiet := Quiet
	Quiet = quiet

	defer func() {
		Quiet = prevQuiet
	}()

	Println("\nInterpolating command with input arguments...")

	for k, v := range args {
		Printf("  - interpolating [#{%s}] => [%s]\n", k, v)

		if AtomicsFolderRegex.MatchString(v) {
			v = AtomicsFolderRegex.ReplaceAllString(v, "")
			v = strings.ReplaceAll(v, `\`, `/`)
			v = strings.TrimSuffix(base, "/") + "/" + v
		}

		interpolated = strings.ReplaceAll(interpolated, "#{"+k+"}", v)
	}

	return interpolated, nil
}

func executeShell(shellName string, command string, env []string, stage string, technique string, testName string, runSpec *RunSpec) (string, error) {
	 Printf("\nExecuting executor=%s command=[%s]\n", shellName, command)

	f, err := os.Create(runSpec.TempDir + "/goart-" + technique + "-" + stage + "." + shellName)
	if err != nil {
		return "", fmt.Errorf("creating temporary file: %w", err)
	}

	if _, err := f.Write([]byte(command)); err != nil {
		f.Close()

		return "", fmt.Errorf("writing command to file: %w", err)
	}

	if err := f.Close(); err != nil {
		return "", fmt.Errorf("closing %s script: %w", shellName, err)
	}

	// guard against hanging tests - kill after a timeout

	timeoutSec := 30*time.Second
	if stage != "test" {
		timeoutSec = 15*time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeoutSec)
	defer cancel()

	cmd := exec.CommandContext(ctx, shellName, f.Name())
	cmd.Env = append(os.Environ(), env...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if context.DeadlineExceeded == ctx.Err() {
			return string(output), fmt.Errorf("TIMED OUT: script %w", err)
		}
		return string(output), fmt.Errorf("executing %s script: %w", shellName, err)
	}
/*
	err = ctx.Err()
	if err != nil {

		if context.DeadlineExceeded == err {
			return string(output), fmt.Errorf("TIMED OUT: script %w", err)
		} else {
			return string(output), fmt.Errorf("ERROR: script %w", err)
		}
	}*/

	return string(output), nil
}
