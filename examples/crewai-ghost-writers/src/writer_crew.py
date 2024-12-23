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

def create_content_crew(topic: str, subtopic_suggestions: list[str]) -> Crew:
    """
    Creates a crew of agents to collaboratively generate a blog post.

    Args:
        topic: The initial topic for the blog post.
        subtopic_suggestions: A list of strings, each representing a subtopic suggestion.

    Returns:
        A Crew instance configured with the agents and tasks.
    """

    # Define roles for each agent
    ideator = Agent(
        role='Ideation Specialist',
        goal=dedent(f'''Develop creative and comprehensive ideas for a blog post on '{topic}'.
                     Consider various angles, subtopics, and potential reader interests.
                     Here are some subtopic suggestions to consider:
                     {chr(10).join([f"- {suggestion}" for suggestion in subtopic_suggestions])}
                     Be flexible in combining several notes into one subtopic, or fleshing out each note as a separate subtopic as necessary for an entertaining blog article.'''),
        backstory=dedent('''As an Ideation Specialist, you excel at brainstorming and expanding
                          on initial concepts. Your expertise lies in generating a wide range of
                          ideas that can captivate and inform readers.'''),
        verbose=True,
        allow_delegation=False,
        llm=myLLM
    )

    writer = Agent(
        role='Content Writer',
        goal=dedent(f'''Write a detailed, engaging, and informative blog post based on the
                     ideas and outline provided. Ensure the content is clear, well-structured,
                     and suitable for a general audience.'''),
        backstory=dedent('''You are a skilled Content Writer known for your ability to transform
                          ideas into compelling narratives. Your writing is characterized by its
                          clarity, depth, and engagement.'''),
        verbose=True,
        allow_delegation=True,
        llm=myLLM
    )

    evaluator = Agent(
        role='Content Evaluator',
        goal=dedent(f'''Critically evaluate the blog post for coherence, depth, originality,
                     and readability. Provide constructive feedback to improve the content's
                     quality and impact.'''),
        backstory=dedent('''As a Content Evaluator, you have a keen eye for detail and a deep
                          understanding of what makes content resonate with readers. Your
                          feedback is invaluable in refining and enhancing the blog post.'''),
        verbose=True,
        allow_delegation=False,
        llm=myLLM
    )
    
    # Define tasks for the agents
    task_ideation = Task(
        description=dedent(f'''Develop a comprehensive set of ideas and an outline for a blog
                             post on '{topic}'. Include potential subheadings, key points to
                             cover, and any relevant examples or anecdotes.'''),
        expected_output='Draft of ideas',
        agent=ideator
    )

    task_writing = Task(
        description=dedent('''Write a full draft of the blog post based on the outline provided.
                             Expand on each point, ensuring the content flows well and is
                             engaging. Aim for a word count of around 350-500 words.'''),
        expected_output='Draft of blog post',
        agent=writer
    )

    # Create the crew with a sequential process
    content_crew = Crew(
        agents=[ideator, writer, evaluator],
        tasks=[task_ideation, task_writing],
        process=Process.sequential,
        verbose=True
    )
    return content_crew

def kickoff_content_crew(file_path: str) -> str:
    """
    Kicks off the content crew and writes the result to the specified output file.

    Args:
        file_path: The path to the file containing subtopic suggestions.

    Returns:
        The result of the crew's execution.
    """
    
    with open(file_path, 'r') as file:
        subtopic_suggestions = [line.strip() for line in file.readlines()]
    topic = os.path.splitext(os.path.basename(file_path))[0].replace('-', ' ').title()
    content_crew = create_content_crew(topic, subtopic_suggestions)
    result = content_crew.kickoff()

    print("result", result)

    return result