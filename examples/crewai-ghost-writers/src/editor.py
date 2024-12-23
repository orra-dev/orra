"""
#  This Source Code Form is subject to the terms of the Mozilla Public
#  License, v. 2.0. If a copy of the MPL was not distributed with this
#  file, You can obtain one at https://mozilla.org/MPL/2.0/.
"""

from crewai import Agent, Task, Crew, Process, LLM
from textwrap import dedent
import os
from dotenv import load_dotenv

load_dotenv()

myLLM = LLM(
    model="gpt-4o",
    temperature=0.7,
    api_key=os.getenv("OPENAI_API_KEY")
)

def create_content_crew(draft: str) -> Crew:
    """
    Creates a crew with an editor agent to revise and improve a blog post draft.

    Args:
        draft: The draft blog post content to be edited and improved.

    Returns:
        A Crew instance configured with an editor agent and editing task.
    """

    editor = Agent(
        role='Content Editor',
        goal=dedent(f'''Implement revisions and improvements to the blog post based on the
                     evaluator's feedback. Focus on enhancing clarity, flow, and impact while
                     maintaining the original message and voice. Address all critiques to
                     elevate the content quality.'''),
        backstory=dedent('''You are a masterful Content Editor with years of experience
                          refining and polishing written content. Your strength lies in
                          taking constructive feedback and skillfully implementing changes
                          that transform good content into exceptional content. You excel at
                          preserving the author's voice while enhancing readability and
                          engagement.'''),
        verbose=True,
        allow_delegation=False,
        llm=myLLM
    )

    task_editing = Task(
        description=dedent(f'''Review the evaluation feedback and implement necessary revisions
                             to improve the blog post.
                             Focus on enhancing clarity, flow, and impact while maintaining 
                             the original voice and message. Address all feedback points systematically 
                             to elevate content quality. Append to the top of the article, five different 
                             suggested titles with a different angle or storytelling principle for each.
                             
                             DRAFT BLOG POST:
                             {draft}
                           '''),
        expected_output='Revised blog post',
        agent=editor
    )

    content_crew = Crew(
        agents=[editor],
        tasks=[task_editing],
        process=Process.sequential,
        verbose=True
    )
    return content_crew

def kickoff_editing_crew(draft: str, output_file_path: str):
    """
    Kicks off the editing crew and writes the result to the specified output file.

    Args:
        draft: The draft blog post content to be edited and improved.
        output_file_path: The path to the file where the edited content will be written.

    Returns:
        The path to the output file where the edited content is written.
    """
    
    content_crew = create_content_crew(draft)
    result = content_crew.kickoff()

    with open(output_file_path, 'w') as output_file:
        output_file.write(str(result))

    return output_file_path

