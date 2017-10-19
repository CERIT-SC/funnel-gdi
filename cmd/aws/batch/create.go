package batch

import (
	"fmt"
	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/logger"
	"github.com/spf13/cobra"
)

func init() {
	f := createCmd.Flags()
	f.StringVar(&funnelConfigFile, "config", funnelConfigFile, "Funnel configuration file")
	f.StringVar(&conf.Region, "region", conf.Region, "Region in which to create the Batch resources")
	f.StringVar(&conf.ComputeEnv.Name, "ComputeEnv.Name", conf.ComputeEnv.Name, "The name of the compute environment.")
	f.Int64Var(&conf.ComputeEnv.MinVCPUs, "ComputEnv.MinVCPUs", conf.ComputeEnv.MinVCPUs, "The minimum number of EC2 vCPUs that an environment should maintain. (default 0)")
	f.Int64Var(&conf.ComputeEnv.MaxVCPUs, "ComputEnv.MaxVCPUs", conf.ComputeEnv.MaxVCPUs, "The maximum number of EC2 vCPUs that an environment can reach.")
	f.StringSliceVar(&conf.ComputeEnv.SecurityGroupIds, "ComputEnv.SecurityGroupIds", conf.ComputeEnv.SecurityGroupIds, "The EC2 security groups that are associated with instances launched in the compute environment. If none are specified all security groups will be used.")
	f.StringSliceVar(&conf.ComputeEnv.Subnets, "ComputEnv.Subnets", conf.ComputeEnv.Subnets, "The VPC subnets into which the compute resources are launched. If none are specified all subnets will be used.")
	f.StringSliceVar(&conf.ComputeEnv.InstanceTypes, "ComputEnv.InstanceTypes", conf.ComputeEnv.InstanceTypes, "The instances types that may be launched. You can also choose optimal to pick instance types on the fly that match the demand of your job queues.")
	f.StringVar(&conf.JobQueue.Name, "JobQueue.Name", conf.JobQueue.Name, "The name of the job queue.")
	f.Int64Var(&conf.JobQueue.Priority, "JobQueue.Priority", conf.JobQueue.Priority, "The priority of the job queue. Priority is determined in descending order.")
	f.StringVar(&conf.JobDef.Name, "JobDef.Name", conf.JobDef.Name, "The name of the job definition.")
	f.StringVar(&conf.JobDef.Image, "JobDef.Image", conf.JobDef.Image, "The docker image used to start a container.")
	f.Int64Var(&conf.JobDef.MemoryMiB, "JobDef.MemoryMiB", conf.JobDef.MemoryMiB, "The hard limit (in MiB) of memory to present to the container.")
	f.Int64Var(&conf.JobDef.VCPUs, "JobDef.VCPUs", conf.JobDef.VCPUs, "The number of vCPUs reserved for the container.")
	f.StringVar(&conf.JobDef.JobRoleArn, "JobDef.JobRoleArn", conf.JobDef.JobRoleArn, "The Amazon Resource Name (ARN) of the IAM role that the container can assume for AWS permissions. A role will be created if not provided.")
}

var createCmd = &cobra.Command{
	Use:   "create-resources",
	Short: "Create a compute environment, job queue and job definition in a specified region",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.NewLogger("batch-create-resources", logger.DefaultConfig())

		if funnelConfigFile != "" {
			funnelConf := config.Config{}
			config.ParseFile(funnelConfigFile, &funnelConf)
			conf.FunnelWorker = funnelConf.Worker
		}

		cli, err := newBatchSvc(conf)
		if err != nil {
			return err
		}

		a, err := cli.CreateComputeEnvironment()
		switch err.(type) {
		case nil:
			log.Info("Created ComputeEnvironment",
				"Name", *a.ComputeEnvironmentName,
				"Arn", *a.ComputeEnvironmentArn,
			)
		case errResourceExists:
			log.Error("ComputeEnvironment already exists",
				"Name", *a.ComputeEnvironmentName,
				"Arn", *a.ComputeEnvironmentArn,
			)
		default:
			return fmt.Errorf("failed to create ComputeEnvironment: %v", err)
		}

		b, err := cli.CreateJobQueue()
		switch err.(type) {
		case nil:
			log.Info("Created JobQueue",
				"Name", *b.JobQueueName,
				"Arn", *b.JobQueueArn,
			)
		case errResourceExists:
			log.Error("JobQueue already exists",
				"Name", *b.JobQueueName,
				"Arn", *b.JobQueueArn,
			)
		default:
			return fmt.Errorf("failed to create JobQueue: %v", err)
		}

		c, err := cli.CreateJobDefinition(false)
		switch err.(type) {
		case nil:
			log.Info("Created JobDefinition",
				"Name", *c.JobDefinitionName,
				"Arn", *c.JobDefinitionArn,
				"Revision", *c.Revision,
			)
		case errResourceExists:
			log.Error("JobDefinition already exists",
				"Name", *c.JobDefinitionName,
				"Arn", *c.JobDefinitionArn,
				"Revision", *c.Revision,
			)
		default:
			return fmt.Errorf("failed to create JobDefinition: %v", err)
		}

		return nil
	},
}
