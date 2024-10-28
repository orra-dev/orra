package main

import (
	"encoding/json"
	"fmt"
)

func generateLLMPrompt(action string, actionParams json.RawMessage, serviceDescriptions string) string {
	prompt := fmt.Sprintf(`You are an AI orchestrator tasked with planning the execution of services based on a user's action. A user's action contains PARAMS for the action to be executed, USE THEM. Your goal is to create an efficient, parallel execution plan that fulfills the user's request.

Available Services:
%s

User Action: %s

Action Params:
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
		- The final task MUST have "final" as the task ID.
		- The final task DOES NOT require a service.
		- The final task input PARAM key should be "error" and the value should explain why the query cannot be addressed.   
		- NO OTHER TASKS ARE REQUIRED. 
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
	)

	return prompt
}
