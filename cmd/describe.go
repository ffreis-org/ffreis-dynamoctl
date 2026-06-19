package cmd

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/spf13/cobra"

	"github.com/ffreis/dynamoctl/internal/output"
)

func newDescribeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "describe",
		Short: "Show table schema, billing mode, and index layout",
		Long: `Describe prints the DynamoDB table's schema and current metadata:
key attributes (PK + SK names/types), billing mode, item count, table size,
and any global secondary indexes.

The --table flag selects the target table (default: DYNAMOCTL_TABLE env or "dynamoctl").`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			client, err := ddbClientFactory(ctx)
			if err != nil {
				return err
			}

			out, err := client.DescribeTable(ctx, &awsdynamodb.DescribeTableInput{
				TableName: aws.String(flagTable),
			})
			if err != nil {
				return fmt.Errorf("describing table %q: %w", flagTable, err)
			}

			tbl := out.Table

			info := output.TableInfo{
				TableName: aws.ToString(tbl.TableName),
				Status:    string(tbl.TableStatus),
			}

			if tbl.BillingModeSummary != nil {
				info.BillingMode = string(tbl.BillingModeSummary.BillingMode)
			} else {
				// nil summary means legacy PROVISIONED mode
				info.BillingMode = "PROVISIONED"
			}
			if tbl.ItemCount != nil {
				info.ItemCount = *tbl.ItemCount
			}
			if tbl.TableSizeBytes != nil {
				info.SizeBytes = *tbl.TableSizeBytes
			}

			// build name→attrType lookup for cross-referencing key schema
			attrTypes := make(map[string]string, len(tbl.AttributeDefinitions))
			for _, a := range tbl.AttributeDefinitions {
				attrTypes[aws.ToString(a.AttributeName)] = string(a.AttributeType)
			}

			for _, k := range tbl.KeySchema {
				name := aws.ToString(k.AttributeName)
				info.KeySchema = append(info.KeySchema, output.KeyAttr{
					Name:     name,
					KeyType:  string(k.KeyType),
					AttrType: attrTypes[name],
				})
			}

			for _, gsi := range tbl.GlobalSecondaryIndexes {
				gv := output.GSIView{
					Name:   aws.ToString(gsi.IndexName),
					Status: string(gsi.IndexStatus),
				}
				if gsi.Projection != nil {
					gv.Projection = string(gsi.Projection.ProjectionType)
				}
				for _, k := range gsi.KeySchema {
					name := aws.ToString(k.AttributeName)
					gv.KeySchema = append(gv.KeySchema, output.KeyAttr{
						Name:     name,
						KeyType:  string(k.KeyType),
						AttrType: attrTypes[name],
					})
				}
				info.GSIs = append(info.GSIs, gv)
			}

			p := output.New(cmd.OutOrStdout(), currentOutput(), cliUI)
			return p.PrintDescribeResult(info)
		},
	}
}
