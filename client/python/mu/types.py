# SPDX-License-Identifier: Apache-2.0
"""JSON types for Mu's assistant contract; unknown JSON fields are retained."""
from typing import Any, NotRequired, TypedDict


class Agent(TypedDict):
    id: str
    name: str
    slug: str
    description: str


class Thread(TypedDict):
    id: str
    agentId: str
    subject: str
    started: NotRequired[str]
    updated: NotRequired[str]


class Content(TypedDict):
    fallback: str
    source: NotRequired[dict[str, Any]]
    image: NotRequired[dict[str, Any]]
    audio: NotRequired[dict[str, Any]]
    video: NotRequired[dict[str, Any]]
    route: NotRequired[dict[str, Any]]
    resource: NotRequired[dict[str, Any]]


class Message(TypedDict):
    id: str
    role: str
    text: str
    content: list[Content]
    at: NotRequired[str]


class Request(TypedDict):
    prompt: str
    agentId: str
    thread: str
    mode: str
    voice: NotRequired[bool]


class Submission(TypedDict):
    messageId: str
    request: Request


class Answer(TypedDict):
    text: str
    thread: str
    agentId: str
    content: list[Content]


class Operation(TypedDict):
    id: str
    thread: str
    state: str
    agentId: NotRequired[str]
    result: NotRequired[Answer]
    error: NotRequired[str]
    created: NotRequired[str]


class Agents(TypedDict):
    agents: list[Agent]
    publicQuestions: NotRequired[bool]


class Threads(TypedDict):
    threads: list[Thread]
    nextPageToken: str


class History(TypedDict):
    thread: Thread
    messages: list[Message]
    nextPageToken: str
    pending: bool
