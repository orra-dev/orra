/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// generateDomainContext creates a Domain Context string from a slice of GroundingUseCase.
// It uses a template-based transformation:
//   - If the Action field contains inline placeholders (e.g., "{orderId}"), it replaces each occurrence
//     with the corresponding value from Params.
//   - Otherwise, if Params is provided, it generates an "Example" line by joining key-value pairs.
func generateDomainContext(useCases []GroundingUseCase, constraints []string) string {
	if len(useCases) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Domain Context:\n")
	for i, uc := range useCases {
		// Print the action
		sb.WriteString(fmt.Sprintf("%d. Action: \"%s\"\n", i+1, uc.Action))
		var exampleLine string
		// Check if the Action string contains placeholders indicated by curly braces.
		if strings.Contains(uc.Action, "{") && strings.Contains(uc.Action, "}") {
			// Copy the action and replace the placeholders with parameter values.
			exampleLine = uc.Action
			for key, val := range uc.Params {
				placeholder := fmt.Sprintf("{%s}", key)
				exampleLine = strings.ReplaceAll(exampleLine, placeholder, val)
			}
		} else if len(uc.Params) > 0 {
			// Provide a generic example by listing all parameters.
			var parts []string
			for key, val := range uc.Params {
				parts = append(parts, fmt.Sprintf("%s \"%s\"", key, val))
			}
			exampleLine = "For " + strings.Join(parts, ", ")
		} else {
			// No parameters to show.
			exampleLine = "No parameters provided"
		}
		sb.WriteString(fmt.Sprintf("   - Example: %s\n", exampleLine))
		// List the capabilities, if any.
		if len(uc.Capabilities) > 0 {
			capStr := strings.Join(uc.Capabilities, ", ")
			sb.WriteString(fmt.Sprintf("   - Capabilities: %s\n", capStr))
		}
		// Finally, output the intent.
		sb.WriteString(fmt.Sprintf("   - Intent: %s\n", uc.Intent))
	}
	if len(constraints) == 0 {
		return sb.String()
	}

	sb.WriteString("Constraints:\n")
	for _, constraint := range constraints {
		sb.WriteString(fmt.Sprintf("- %s\n", constraint))
	}
	return sb.String()
}

func generatePlannerPrompt(action string, actionParams json.RawMessage, serviceDescriptions string, grounding *GroundingSpec) string {
	var (
		useCases    []GroundingUseCase
		constraints []string
	)

	if grounding != nil {
		useCases = grounding.UseCases
		constraints = grounding.Constraints
	}

	prompt := fmt.Sprintf(`You are an AI orchestrator tasked with planning the execution of services based on a user's action. A user's action contains PARAMS for the action to be executed, USE THEM. Your goal is to create an efficient, parallel execution plan that fulfills the user's request.

Available Services:
%s

User Action: %s

Action Params:
%s

%s
Guidelines:
1. Each service described above contains input/output types and description. You must strictly adhere to these types and descriptions when using the services.
2. Each task in the plan should strictly use one of the available services. Follow the JSON conventions for each task.
3. Each task MUST have a unique ID, which is strictly increasing.
4. With the excpetion of Task 0, whose inputs are constants derived from the User Action, inputs for other tasks have to be outputs from preceding tasks. In the latter case, use the format $taskId to denote the ID of the previous task whose output will be the input.
5. There can only be a single Task 0, other tasks HAVE TO CORRESPOND TO AVAILABLE SERVICES.
6. Ensure the plan maximizes parallelizability.
7. Only use the provided services.
	- If a query cannot be addressed using these, USE A "final" TASK TO SUGGEST THE NEXT STEPS.
		- The final task MUST have "final" as the task ID: { "id": "final".
		- The final task DOES NOT require a service.
		- The final task input PARAM key should be "error" and the value should explain why the query cannot be addressed.   
		- EXCEPT FOR TASK 0, NO OTHER TASKS ARE REQUIRED AND SHOULD BE REMOVED. 
8. Never explain the plan with comments.
9. Never introduce new services other than the ones provided.

Please generate a plan in the following JSON format:

{
  "tasks": [
    {
      "id": "task0",
      "input": {
        "param1": "value1"
      }
    },
    {
      "id": "task1",
      "service": "ServiceID",
      "input": {
        "param1": "$task0.param1"
      }
    },
    {
      "id": "task2",
      "service": "AnotherServiceID",
      "input": {
        "param1": "$task1.param1"
      }
    }
  ],
  "parallel_groups": [
    ["task1"],
    ["task2"]
  ]
}

Ensure that the plan is efficient, maximizes parallelization, and accurately fulfills the user's action using the available services. If the action cannot be completed with the given services, explain why in a "final" task and suggest alternatives if possible.

Generate the execution plan:`,
		serviceDescriptions,
		action,
		string(actionParams),
		generateDomainContext(useCases, constraints),
	)

	return prompt
}
