package terraform

import (
	"strings"
	"testing"

	"github.com/os-tack/fcp-terraform/internal/fcpcore"
)

func TestNovaPay(t *testing.T) {
	model := NewModel("NovaPay")

	// Helper to run an op
	runOp := func(opStr string) string {
		result := fcpcore.ParseOp(opStr)
		if result.IsError() {
			t.Fatalf("parse error for %q: %s", opStr, result.Err.Error)
		}
		return Dispatch(result.Op, model)
	}

	// Helper to check op succeeded (starts with + or * or ~ or @ or -)
	runOps := func(ops ...string) {
		for _, op := range ops {
			res := runOp(op)
			if strings.HasPrefix(res, "ERROR") {
				t.Fatalf("op failed: %q -> %s", op, res)
			}
		}
	}

	// === ROUND 1 ===
	t.Run("Round1", func(t *testing.T) {
		runOps(
			// Provider
			`add provider aws region:us-east-1`,

			// Variables (8)
			`add variable environment type:string default:production description:"Deployment environment"`,
			`add variable project_name type:string default:novapay description:"Project name for resource naming"`,
			`add variable vpc_cidr type:string default:"10.0.0.0/16" description:"VPC CIDR block"`,
			`add variable ecs_task_cpu type:number default:256 description:"ECS task CPU units"`,
			`add variable ecs_task_memory type:number default:512 description:"ECS task memory (MiB)"`,
			`add variable db_engine_version type:string default:s:15.4 description:"PostgreSQL engine version"`,
			`add variable enable_deletion_protection type:bool default:true description:"Enable deletion protection on RDS"`,
			`add variable container_port type:number default:8080 description:"Container port for the API service"`,

			// Data sources (2)
			`add data aws_caller_identity current`,
			`add data aws_availability_zones available state:"available"`,

			// --- Networking (10) ---
			`add resource aws_vpc main cidr_block:var.vpc_cidr enable_dns_support:true enable_dns_hostnames:true`,
			`tag aws_vpc.main Name:"${var.project_name}-vpc" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_subnet private_a vpc_id:aws_vpc.main.id cidr_block:"10.0.1.0/24" availability_zone:data.aws_availability_zones.available.names[0]`,
			`tag private_a Name:"${var.project_name}-private-a" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_subnet private_b vpc_id:aws_vpc.main.id cidr_block:"10.0.2.0/24" availability_zone:data.aws_availability_zones.available.names[1]`,
			`tag private_b Name:"${var.project_name}-private-b" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_subnet public_a vpc_id:aws_vpc.main.id cidr_block:"10.0.10.0/24" availability_zone:data.aws_availability_zones.available.names[0] map_public_ip_on_launch:true`,
			`tag public_a Name:"${var.project_name}-public-a" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_subnet public_b vpc_id:aws_vpc.main.id cidr_block:"10.0.11.0/24" availability_zone:data.aws_availability_zones.available.names[1] map_public_ip_on_launch:true`,
			`tag public_b Name:"${var.project_name}-public-b" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_internet_gateway main vpc_id:aws_vpc.main.id`,
			`tag aws_internet_gateway.main Name:"${var.project_name}-igw" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_eip nat domain:vpc`,
			`tag nat Name:"${var.project_name}-nat-eip" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_nat_gateway main allocation_id:aws_eip.nat.id subnet_id:aws_subnet.public_a.id`,
			`tag aws_nat_gateway.main Name:"${var.project_name}-nat" Environment:var.environment ManagedBy:terraform`,

			// Route tables with nested route blocks
			`add resource aws_route_table public vpc_id:aws_vpc.main.id`,
			`nest aws_route_table.public route cidr_block:"0.0.0.0/0" gateway_id:aws_internet_gateway.main.id`,
			`tag aws_route_table.public Name:"${var.project_name}-public-rt" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_route_table private vpc_id:aws_vpc.main.id`,
			`nest aws_route_table.private route cidr_block:"0.0.0.0/0" nat_gateway_id:aws_nat_gateway.main.id`,
			`tag aws_route_table.private Name:"${var.project_name}-private-rt" Environment:var.environment ManagedBy:terraform`,

			// --- Security Groups (4) ---
			`add resource aws_security_group alb name_prefix:"${var.project_name}-alb-" vpc_id:aws_vpc.main.id`,
			`nest alb ingress from_port:80 to_port:80 protocol:tcp cidr_blocks:["0.0.0.0/0"]`,
			`nest alb ingress from_port:443 to_port:443 protocol:tcp cidr_blocks:["0.0.0.0/0"]`,
			`nest alb egress from_port:0 to_port:0 protocol:"-1" cidr_blocks:["0.0.0.0/0"]`,
			`tag alb Name:"${var.project_name}-alb-sg" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_security_group ecs name_prefix:"${var.project_name}-ecs-" vpc_id:aws_vpc.main.id`,
			`nest ecs ingress from_port:var.container_port to_port:var.container_port protocol:tcp security_groups:[aws_security_group.alb.id]`,
			`nest ecs egress from_port:0 to_port:0 protocol:"-1" cidr_blocks:["0.0.0.0/0"]`,
			`tag ecs Name:"${var.project_name}-ecs-sg" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_security_group db name_prefix:"${var.project_name}-db-" vpc_id:aws_vpc.main.id`,
			`nest db ingress from_port:5432 to_port:5432 protocol:tcp security_groups:[aws_security_group.ecs.id]`,
			`nest db egress from_port:0 to_port:0 protocol:"-1" cidr_blocks:["0.0.0.0/0"]`,
			`tag db Name:"${var.project_name}-db-sg" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_security_group cache name_prefix:"${var.project_name}-cache-" vpc_id:aws_vpc.main.id`,
			`nest cache ingress from_port:6379 to_port:6379 protocol:tcp security_groups:[aws_security_group.ecs.id]`,
			`nest cache egress from_port:0 to_port:0 protocol:"-1" cidr_blocks:["0.0.0.0/0"]`,
			`tag cache Name:"${var.project_name}-cache-sg" Environment:var.environment ManagedBy:terraform`,

			// --- Load Balancer (3) ---
			`add resource aws_lb main name:"${var.project_name}-alb" load_balancer_type:application internal:false subnets:[aws_subnet.public_a.id,aws_subnet.public_b.id] security_groups:[aws_security_group.alb.id]`,
			`tag aws_lb.main Name:"${var.project_name}-alb" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_lb_target_group api name:"${var.project_name}-api-tg" port:var.container_port protocol:HTTP vpc_id:aws_vpc.main.id target_type:ip`,
			`nest api health_check path:"/health" protocol:HTTP`,
			`tag api Name:"${var.project_name}-api-tg" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_lb_listener http load_balancer_arn:aws_lb.main.arn port:80 protocol:HTTP`,
			`nest http default_action type:forward target_group_arn:aws_lb_target_group.api.arn`,

			// --- Compute (5) ---
			`add resource aws_ecs_cluster main name:"${var.project_name}-${var.environment}"`,
			`tag aws_ecs_cluster.main Name:"${var.project_name}-ecs-cluster" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_iam_role ecs_execution name:"${var.project_name}-ecs-execution"`,
			`tag ecs_execution Name:"${var.project_name}-ecs-execution-role" Environment:var.environment ManagedBy:terraform`,
		)

		// Set jsonencode expression for assume_role_policy using positional expression syntax
		runOp(`set ecs_execution assume_role_policy "jsonencode({Version = \"2012-10-17\", Statement = [{Effect = \"Allow\", Principal = {Service = \"ecs-tasks.amazonaws.com\"}, Action = \"sts:AssumeRole\"}]})"`)

		runOps(
			`add resource aws_iam_role_policy_attachment ecs_execution role:aws_iam_role.ecs_execution.name policy_arn:"arn:aws:iam::policy/service-role/AmazonECSTaskExecutionRolePolicy"`,

			`add resource aws_ecs_task_definition api family:"${var.project_name}-api" requires_compatibilities:["FARGATE"] network_mode:awsvpc cpu:var.ecs_task_cpu memory:var.ecs_task_memory execution_role_arn:aws_iam_role.ecs_execution.arn`,
		)

		// Set jsonencode expression for container_definitions
		runOp(`set aws_ecs_task_definition.api container_definitions "jsonencode([{name = \"api\", image = \"nginx:latest\", essential = true, portMappings = [{containerPort = var.container_port, protocol = \"tcp\"}]}])"`)

		runOps(
			`tag aws_ecs_task_definition.api Name:"${var.project_name}-api-task" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_ecs_service api name:"${var.project_name}-api" cluster:aws_ecs_cluster.main.id task_definition:aws_ecs_task_definition.api.arn desired_count:2 launch_type:FARGATE`,
			`nest aws_ecs_service.api network_configuration subnets:[aws_subnet.private_a.id,aws_subnet.private_b.id] security_groups:[aws_security_group.ecs.id] assign_public_ip:false`,
			`nest aws_ecs_service.api load_balancer target_group_arn:aws_lb_target_group.api.arn container_name:api container_port:var.container_port`,
			`tag aws_ecs_service.api Name:"${var.project_name}-ecs-service" Environment:var.environment ManagedBy:terraform`,

			// --- Data Tier (3) ---
			`add resource aws_db_subnet_group main name:"${var.project_name}-db-subnet" subnet_ids:[aws_subnet.private_a.id,aws_subnet.private_b.id]`,
			`tag aws_db_subnet_group.main Name:"${var.project_name}-db-subnet-group" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_db_instance main engine:postgres engine_version:var.db_engine_version instance_class:"db.t3.medium" allocated_storage:50 storage_type:gp3 db_name:novapay db_subnet_group_name:aws_db_subnet_group.main.name vpc_security_group_ids:[aws_security_group.db.id] skip_final_snapshot:true deletion_protection:var.enable_deletion_protection`,
			`tag aws_db_instance.main Name:"${var.project_name}-db" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_elasticache_cluster redis cluster_id:"${var.project_name}-redis" engine:redis node_type:"cache.t3.micro" num_cache_nodes:1 security_group_ids:[aws_security_group.cache.id]`,
			`tag redis Name:"${var.project_name}-redis" Environment:var.environment ManagedBy:terraform`,

			// --- Messaging & Storage (2) ---
			`add resource aws_sqs_queue events name:"${var.project_name}-events-${var.environment}" visibility_timeout_seconds:300`,
			`tag events Name:"${var.project_name}-events-queue" Environment:var.environment ManagedBy:terraform`,

			`add resource aws_s3_bucket artifacts bucket:"${var.project_name}-artifacts-${var.environment}"`,
			`tag artifacts Name:"${var.project_name}-artifacts" Environment:var.environment ManagedBy:terraform`,

			// --- Outputs (6) ---
			`add output vpc_id value:aws_vpc.main.id`,
			`add output alb_dns_name value:aws_lb.main.dns_name`,
			`add output ecs_cluster_name value:aws_ecs_cluster.main.name`,
			`add output db_endpoint value:aws_db_instance.main.endpoint`,
			`add output sqs_queue_url value:aws_sqs_queue.events.url`,
			`add output s3_bucket_arn value:aws_s3_bucket.artifacts.arn`,
		)

		hcl := string(model.Bytes())

		// Scorecard Round 1
		// #1 Provider
		assert(t, hcl, `provider "aws"`, "R1#1 Provider block")
		assertAttr(t, hcl, `region`, `"us-east-1"`, "R1#1 Provider region")

		// #2 Variables (8)
		assert(t, hcl, `variable "environment"`, "R1#2 environment var")
		assert(t, hcl, `variable "project_name"`, "R1#2 project_name var")
		assert(t, hcl, `variable "vpc_cidr"`, "R1#2 vpc_cidr var")
		assert(t, hcl, `variable "ecs_task_cpu"`, "R1#2 ecs_task_cpu var")
		assert(t, hcl, `variable "ecs_task_memory"`, "R1#2 ecs_task_memory var")
		assert(t, hcl, `variable "db_engine_version"`, "R1#2 db_engine_version var")
		assert(t, hcl, `variable "enable_deletion_protection"`, "R1#2 enable_deletion_protection var")
		assert(t, hcl, `variable "container_port"`, "R1#2 container_port var")

		// #3-4 Data sources
		assert(t, hcl, `data "aws_caller_identity" "current"`, "R1#3 caller identity")
		assert(t, hcl, `data "aws_availability_zones" "available"`, "R1#4 AZs")

		// #5 VPC
		assert(t, hcl, `resource "aws_vpc" "main"`, "R1#5 VPC")
		assertAttr(t, hcl, `cidr_block`, `var.vpc_cidr`, "R1#5 VPC CIDR from variable")
		assert(t, hcl, `enable_dns_support`, "R1#5 DNS support")
		assert(t, hcl, `enable_dns_hostnames`, "R1#5 DNS hostnames")

		// #6 Subnets
		assert(t, hcl, `resource "aws_subnet" "private_a"`, "R1#6 private_a subnet")
		assert(t, hcl, `resource "aws_subnet" "private_b"`, "R1#6 private_b subnet")
		assert(t, hcl, `resource "aws_subnet" "public_a"`, "R1#6 public_a subnet")
		assert(t, hcl, `resource "aws_subnet" "public_b"`, "R1#6 public_b subnet")

		// #7 Internet gateway
		assert(t, hcl, `resource "aws_internet_gateway" "main"`, "R1#7 IGW")

		// #8 NAT gateway
		assert(t, hcl, `resource "aws_nat_gateway" "main"`, "R1#8 NAT GW")
		assert(t, hcl, `resource "aws_eip" "nat"`, "R1#8 NAT EIP")

		// #9 Route tables
		assert(t, hcl, `resource "aws_route_table" "public"`, "R1#9 public RT")
		assert(t, hcl, `resource "aws_route_table" "private"`, "R1#9 private RT")

		// #10 Security groups
		assert(t, hcl, `resource "aws_security_group" "alb"`, "R1#10 ALB SG")
		assert(t, hcl, `resource "aws_security_group" "ecs"`, "R1#10 ECS SG")
		assert(t, hcl, `resource "aws_security_group" "db"`, "R1#10 DB SG")
		assert(t, hcl, `resource "aws_security_group" "cache"`, "R1#10 Cache SG")

		// #11 ALB
		assert(t, hcl, `resource "aws_lb" "main"`, "R1#11 ALB")
		assert(t, hcl, `load_balancer_type`, "R1#11 ALB type")

		// #12 Target group
		assert(t, hcl, `resource "aws_lb_target_group" "api"`, "R1#12 Target group")
		assert(t, hcl, `health_check`, "R1#12 Health check")

		// #13 HTTP listener
		assert(t, hcl, `resource "aws_lb_listener" "http"`, "R1#13 HTTP listener")

		// #14 ECS cluster
		assert(t, hcl, `resource "aws_ecs_cluster" "main"`, "R1#14 ECS cluster")

		// #15 IAM role
		assert(t, hcl, `resource "aws_iam_role" "ecs_execution"`, "R1#15 IAM role")

		// #20 IAM role with jsonencode
		assertAttr(t, hcl, `assume_role_policy`, `jsonencode(`, "R1#20 assume_role_policy expression")
		assertNot(t, hcl, `assume_role_policy = "jsonencode`, "R1#20 NOT quoted string")

		// #21 Task definition
		assert(t, hcl, `resource "aws_ecs_task_definition" "api"`, "R1#21 Task def")

		// #22 Task def with jsonencode
		assertAttr(t, hcl, `container_definitions`, `jsonencode(`, "R1#22 container_definitions expression")

		// #23 ECS service
		assert(t, hcl, `resource "aws_ecs_service" "api"`, "R1#23 ECS service")
		assert(t, hcl, `network_configuration`, "R1#23 network config")
		assert(t, hcl, `load_balancer`, "R1#23 load balancer")

		// #24 DB subnet group
		assert(t, hcl, `resource "aws_db_subnet_group" "main"`, "R1#24 DB subnet group")

		// #25 DB instance + engine_version string
		assert(t, hcl, `resource "aws_db_instance" "main"`, "R1#25 DB instance")
		assert(t, hcl, `engine_version`, "R1#25 engine_version")

		// #25 engine_version as string (via s: prefix)
		assertAttr(t, hcl, `default`, `"15.4"`, "R1#25 engine_version string default")

		// #26 ElastiCache
		assert(t, hcl, `resource "aws_elasticache_cluster" "redis"`, "R1#26 ElastiCache")

		// #27 SQS
		assert(t, hcl, `resource "aws_sqs_queue" "events"`, "R1#27 SQS queue")

		// #28 S3
		assert(t, hcl, `resource "aws_s3_bucket" "artifacts"`, "R1#28 S3 bucket")

		// #29 Outputs
		assert(t, hcl, `output "vpc_id"`, "R1#29 vpc_id output")
		assert(t, hcl, `output "alb_dns_name"`, "R1#29 alb_dns_name output")
		assert(t, hcl, `output "ecs_cluster_name"`, "R1#29 ecs_cluster_name output")
		assert(t, hcl, `output "db_endpoint"`, "R1#29 db_endpoint output")
		assert(t, hcl, `output "sqs_queue_url"`, "R1#29 sqs_queue_url output")
		assert(t, hcl, `output "s3_bucket_arn"`, "R1#29 s3_bucket_arn output")

		// #30 Tags on resources
		assert(t, hcl, `ManagedBy`, "R1#30 ManagedBy tag present")
		assert(t, hcl, `Environment`, "R1#30 Environment tag present")

		// #31 Valid HCL syntax
		assertAttr(t, hcl, `cidr_blocks`, `["0.0.0.0/0"]`, "R1#31 cidr_blocks list")
		assertAttr(t, hcl, `security_groups`, `[aws_security_group.`, "R1#31 security_groups list")
		assertAttr(t, hcl, `subnets`, `[aws_subnet.`, "R1#31 subnets list")
	})

	// === ROUND 2 ===
	t.Run("Round2", func(t *testing.T) {
		// #1 Rename ECS service
		runOp(`label aws_ecs_service.api main_api`)

		// #2 HTTPS listener
		runOps(
			`add resource aws_lb_listener https load_balancer_arn:aws_lb.main.arn port:443 protocol:HTTPS ssl_policy:"ELBSecurityPolicy-TLS13-1-2-2021-06" certificate_arn:"arn:aws:acm:us-east-1:123456789012:certificate/example-cert"`,
			`nest https default_action type:forward target_group_arn:aws_lb_target_group.api.arn`,
		)

		// #3 Auto scaling (tests nested nesting)
		runOps(
			`add resource aws_appautoscaling_target ecs max_capacity:10 min_capacity:2 resource_id:"service/${aws_ecs_cluster.main.name}/${aws_ecs_service.main_api.name}" scalable_dimension:"ecs:service:DesiredCount" service_namespace:ecs`,
			`add resource aws_appautoscaling_policy cpu name:cpu-scaling policy_type:TargetTrackingScaling resource_id:aws_appautoscaling_target.ecs.resource_id scalable_dimension:aws_appautoscaling_target.ecs.scalable_dimension service_namespace:aws_appautoscaling_target.ecs.service_namespace`,
			`nest cpu target_tracking_scaling_policy_configuration target_value:70.0`,
			`nest cpu target_tracking_scaling_policy_configuration/predefined_metric_specification predefined_metric_type:ECSServiceAverageCPUUtilization`,
		)

		// #4 Change ecs_task_memory default
		runOp(`set ecs_task_memory default:1024`)

		// #5 SSH ingress on ECS SG (must use qualified name since "ecs" is now ambiguous)
		runOp(`nest aws_security_group.ecs ingress from_port:22 to_port:22 protocol:tcp cidr_blocks:[var.vpc_cidr]`)

		// #6 Replace SQS queue
		runOps(
			`remove events`,
			`add resource aws_sqs_queue orders name:"${var.project_name}-orders-${var.environment}.fifo" fifo_queue:true content_based_deduplication:true`,
			`tag orders Name:"${var.project_name}-orders-queue" Environment:var.environment ManagedBy:terraform`,
			`set sqs_queue_url value:aws_sqs_queue.orders.url`,
		)

		// #7 CloudWatch log group
		runOps(
			`add resource aws_cloudwatch_log_group ecs name:"/ecs/${var.project_name}-api" retention_in_days:30`,
			`tag aws_cloudwatch_log_group.ecs Name:"${var.project_name}-ecs-logs" Environment:var.environment ManagedBy:terraform`,
		)

		// #8 Lifecycle block with ignore_changes (bare identifiers render raw)
		runOp(`nest main_api lifecycle ignore_changes:[desired_count]`)

		// #9 S3 bucket versioning
		runOps(
			`add resource aws_s3_bucket_versioning artifacts bucket:aws_s3_bucket.artifacts.id`,
			`nest aws_s3_bucket_versioning.artifacts versioning_configuration status:Enabled`,
		)

		// #10 API endpoint output
		runOp(`add output api_endpoint value:"https://${aws_lb.main.dns_name}"`)

		hcl := string(model.Bytes())

		// Verify Round 2 items
		assert(t, hcl, `resource "aws_ecs_service" "main_api"`, "R2#1 renamed service")
		assert(t, hcl, `resource "aws_lb_listener" "https"`, "R2#2 HTTPS listener")
		assert(t, hcl, `predefined_metric_specification`, "R2#3 nested metric spec")
		assertAttr(t, hcl, `default`, `1024`, "R2#4 memory changed")
		assertAttr(t, hcl, `from_port`, `22`, "R2#5 SSH ingress")
		assert(t, hcl, `resource "aws_sqs_queue" "orders"`, "R2#6 orders queue")
		assert(t, hcl, `fifo_queue`, "R2#6 FIFO")
		assertAttr(t, hcl, `retention_in_days`, `30`, "R2#7 log retention")
		assertAttr(t, hcl, `ignore_changes`, `[desired_count]`, "R2#8 lifecycle")
		assert(t, hcl, `versioning_configuration`, "R2#9 versioning")
	})

	// === ROUND 3 ===
	t.Run("Round3", func(t *testing.T) {
		// #1 Alert email variable
		runOp(`add variable alert_email type:string default:"ops@novapay.com" description:"Email for alarm notifications"`)

		// #2 SNS topic + subscription
		runOps(
			`add resource aws_sns_topic alerts name:"${var.project_name}-alerts-${var.environment}"`,
			`tag alerts Name:"${var.project_name}-alerts" Environment:var.environment ManagedBy:terraform`,
			`add resource aws_sns_topic_subscription email topic_arn:aws_sns_topic.alerts.arn protocol:email endpoint:var.alert_email`,
		)

		// #3 ECS CPU alarm
		runOps(
			`add resource aws_cloudwatch_metric_alarm ecs_cpu alarm_name:"${var.project_name}-ecs-cpu-high" comparison_operator:GreaterThanThreshold evaluation_periods:2 metric_name:CPUUtilization namespace:"AWS/ECS" period:300 statistic:Average threshold:80 alarm_actions:[aws_sns_topic.alerts.arn]`,
			`tag ecs_cpu Name:"${var.project_name}-ecs-cpu-alarm" Environment:var.environment ManagedBy:terraform`,
		)

		// #4 DB connections alarm
		runOps(
			`add resource aws_cloudwatch_metric_alarm db_connections alarm_name:"${var.project_name}-db-connections-high" comparison_operator:GreaterThanThreshold evaluation_periods:2 metric_name:DatabaseConnections namespace:"AWS/RDS" period:300 statistic:Average threshold:100 alarm_actions:[aws_sns_topic.alerts.arn]`,
			`tag db_connections Name:"${var.project_name}-db-connections-alarm" Environment:var.environment ManagedBy:terraform`,
		)

		// #5 KMS key + alias
		runOps(
			`add resource aws_kms_key rds description:"KMS key for RDS encryption" enable_key_rotation:true`,
			`tag rds Name:"${var.project_name}-rds-kms" Environment:var.environment ManagedBy:terraform`,
			`add resource aws_kms_alias rds name:"alias/${var.project_name}-rds" target_key_id:aws_kms_key.rds.id`,
		)

		// #6 Enable RDS encryption
		runOp(`set aws_db_instance.main storage_encrypted:true kms_key_id:aws_kms_key.rds.arn`)

		// #7 IAM policy with jsonencode S3 access
		runOps(
			`add resource aws_iam_policy ecs_s3_access name:"${var.project_name}-ecs-s3-access"`,
		)
		runOp(`set ecs_s3_access policy "jsonencode({Version = \"2012-10-17\", Statement = [{Effect = \"Allow\", Action = [\"s3:GetObject\", \"s3:PutObject\"], Resource = \"${aws_s3_bucket.artifacts.arn}/*\"}]})"`)
		runOps(
			`tag ecs_s3_access Name:"${var.project_name}-ecs-s3-policy" Environment:var.environment ManagedBy:terraform`,
			`add resource aws_iam_role_policy_attachment ecs_s3 role:aws_iam_role.ecs_execution.name policy_arn:aws_iam_policy.ecs_s3_access.arn`,
		)

		// #8 Monitoring summary output
		runOp(`add output monitoring_summary value:"Alerts: ${aws_sns_topic.alerts.arn}, CPU Alarm: ${aws_cloudwatch_metric_alarm.ecs_cpu.arn}, DB Alarm: ${aws_cloudwatch_metric_alarm.db_connections.arn}"`)

		hcl := string(model.Bytes())

		// Verify Round 3 items
		assert(t, hcl, `variable "alert_email"`, "R3#1 alert email var")
		assert(t, hcl, `resource "aws_sns_topic" "alerts"`, "R3#2 SNS topic")
		assert(t, hcl, `resource "aws_sns_topic_subscription" "email"`, "R3#2 SNS subscription")
		assert(t, hcl, `resource "aws_cloudwatch_metric_alarm" "ecs_cpu"`, "R3#3 CPU alarm")
		assertAttr(t, hcl, `threshold`, `80`, "R3#3 threshold")
		assert(t, hcl, `alarm_actions`, "R3#3 alarm actions")
		assert(t, hcl, `resource "aws_cloudwatch_metric_alarm" "db_connections"`, "R3#4 DB alarm")
		assert(t, hcl, `resource "aws_kms_key" "rds"`, "R3#5 KMS key")
		assert(t, hcl, `resource "aws_kms_alias" "rds"`, "R3#5 KMS alias")
		assertAttr(t, hcl, `storage_encrypted`, `true`, "R3#6 encryption enabled")
		assert(t, hcl, `kms_key_id`, "R3#6 KMS key ref")
		assertAttr(t, hcl, `policy`, `jsonencode(`, "R3#7 IAM policy expression")
		assert(t, hcl, `output "monitoring_summary"`, "R3#8 monitoring output")
	})
}

// Helper assertion functions
func assert(t *testing.T, hcl, pattern, item string) {
	t.Helper()
	if !strings.Contains(hcl, pattern) {
		t.Errorf("[FAIL] %s: expected %q in output", item, pattern)
	}
}

func assertNot(t *testing.T, hcl, pattern, item string) {
	t.Helper()
	if strings.Contains(hcl, pattern) {
		t.Errorf("[FAIL] %s: unexpected %q in output", item, pattern)
	}
}

// assertAttr checks that an HCL attribute line exists with the given key and value,
// tolerating hclwrite's alignment padding (extra spaces around =).
func assertAttr(t *testing.T, hcl, key, value, item string) {
	t.Helper()
	for _, line := range strings.Split(hcl, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key) {
			rest := strings.TrimPrefix(trimmed, key)
			rest = strings.TrimSpace(rest)
			if strings.HasPrefix(rest, "=") {
				rest = strings.TrimPrefix(rest, "=")
				rest = strings.TrimSpace(rest)
				if strings.HasPrefix(rest, value) {
					return // found
				}
			}
		}
	}
	t.Errorf("[FAIL] %s: expected attribute %s = %s in output", item, key, value)
}
