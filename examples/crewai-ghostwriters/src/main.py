"""
#  This Source Code Form is subject to the terms of the Mozilla Public
#  License, v. 2.0. If a copy of the MPL was not distributed with this
#  file, You can obtain one at https://mozilla.org/MPL/2.0/.
"""

import asyncio
import os

from dotenv import load_dotenv
from pydantic import BaseModel

from editor import kickoff_editing_crew
from orra import OrraAgent, Task, RevertSource, CompensationResult, CompensationStatus
from writer_crew import kickoff_content_crew

load_dotenv()

ORRA_APIKEY = os.getenv("ORRA_API_KEY")
DEMO_REVERT_FAIL = os.getenv("DEMO_REVERT_FAIL")
DEMO_ABORT = os.getenv("DEMO_ABORT")


# Define your models
class WriterInput(BaseModel):
    topics_file_path: str


class WriterOutput(BaseModel):
    draft: str


class EditorInput(BaseModel):
    draft: str
    output_path: str


class EditorOutput(BaseModel):
    final_output_path: str


# Configure and run the Agents
async def main():
    writer = OrraAgent(
        name="writer-agent",
        description="Writes a blog post based on a list of topic ideas",
        url="http://localhost:8005",
        api_key=ORRA_APIKEY,
        log_level="DEBUG",
        revertible=True,
    )

    @writer.revert_handler()
    async def revert_draft(source: RevertSource[WriterInput, WriterOutput]) -> CompensationResult:
        print(f"Reverting draft original_task.input: {source.input.topics_file_path}")
        print(f"Reverting draft generated draft: {source.output.draft}")

        if source.context and source.context.reason == "aborted":
            # The abort payload is available in context.payload
            abort_info = source.context.payload
            print(f"Task was aborted for op: {abort_info.get('operation')}")
            print(f"Abort reason: {abort_info.get('reason')}")

        if DEMO_REVERT_FAIL == "true":
            raise RuntimeError('Demo: failed to revert draft.')

        return CompensationResult(status=CompensationStatus.COMPLETED)

    @writer.handler()
    async def write_draft(request: Task[WriterInput]) -> WriterOutput:
        writer_result = kickoff_content_crew(file_path=request.input.topics_file_path)
        return WriterOutput(draft=writer_result.raw)

    editor = OrraAgent(
        name="editor-agent",
        description="Edits the tone of a blog post for improved readability",
        url="http://localhost:8005",
        api_key=ORRA_APIKEY,
        log_level="DEBUG",
    )

    @editor.handler()
    async def edit_draft(request: Task[EditorInput]) -> EditorOutput:
        draft_file_path = kickoff_editing_crew(draft=request.input.draft, output_file_path=request.input.output_path)
        if DEMO_ABORT == "true":
            await request.abort({
                "operation": "edit-draft",
                "reason": "hated the drafted story"
            })
        return EditorOutput(final_output_path=draft_file_path)

    await asyncio.gather(writer.start(), editor.start())

    try:
        await asyncio.get_running_loop().create_future()
    except KeyboardInterrupt:
        await asyncio.gather(
            writer.shutdown(),
            editor.shutdown()
        )


if __name__ == "__main__":
    asyncio.run(main())
