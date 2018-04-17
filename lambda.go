package corral

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/viper"

	"github.com/bcongdon/corral/internal/pkg/corfs"
	"github.com/bcongdon/corral/internal/pkg/corlambda"
)

var (
	currentJob *Job
)

// runningInLambda infers if the program is running in AWS lambda via inspection of the environment
func runningInLambda() bool {
	expectedEnvVars := []string{"LAMBDA_TASK_ROOT", "AWS_EXECUTION_ENV", "LAMBDA_RUNTIME_DIR"}
	for _, envVar := range expectedEnvVars {
		if os.Getenv(envVar) == "" {
			return false
		}
	}
	return true
}

func handleRequest(ctx context.Context, task task) (string, error) {
	fs := corfs.InitFilesystem(task.FileSystemType)
	currentJob.fileSystem = fs
	currentJob.intermediateBins = task.IntermediateBins
	currentJob.outputPath = task.WorkingLocation

	if task.Phase == MapPhase {
		err := currentJob.runMapper(task.BinID, task.Splits)
		return fmt.Sprintf("%v", task), err
	} else if task.Phase == ReducePhase {
		err := currentJob.runReducer(task.BinID)
		return fmt.Sprintf("%v", task), err
	}
	return "", fmt.Errorf("Unknown phase: %d", task.Phase)
}

type lambdaExecutor struct {
	*corlambda.LambdaClient
	functionName string
}

func (l *lambdaExecutor) RunMapper(job *Job, binID uint, inputSplits []inputSplit) error {
	mapTask := task{
		Phase:            MapPhase,
		BinID:            binID,
		Splits:           inputSplits,
		IntermediateBins: job.intermediateBins,
		FileSystemType:   corfs.S3,
		WorkingLocation:  job.config.WorkingLocation,
	}
	payload, err := json.Marshal(mapTask)
	if err != nil {
		return err
	}

	_, err = l.Invoke(l.functionName, payload)
	return err
}

func (l *lambdaExecutor) RunReducer(job *Job, binID uint) error {
	mapTask := task{
		Phase:           ReducePhase,
		BinID:           binID,
		FileSystemType:  corfs.S3,
		WorkingLocation: job.config.WorkingLocation,
	}
	payload, err := json.Marshal(mapTask)
	if err != nil {
		return err
	}

	_, err = l.Invoke(l.functionName, payload)
	return err
}

func (l *lambdaExecutor) Deploy() {
	config := &corlambda.FunctionConfig{
		Name:       l.functionName,
		RoleARN:    viper.GetString("lambdaRoleARN"),
		Timeout:    viper.GetInt64("lambdaTimeout"),
		MemorySize: viper.GetInt64("lambdaMemory"),
	}
	l.DeployFunction(config)
}
