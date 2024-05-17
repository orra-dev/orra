from pydoc import Doc
from typing import Type, Any, TypedDict, Callable, Annotated

import fastapi
from pydantic import BaseModel
from langgraph.graph import StateGraph, END


def _create_typed_dict(name: str, fields: dict[str, Any]) -> Any:
    """
        Create a TypedDict from a dictionary of fields
    """
    return TypedDict(name, fields)


def _create_response_model(typed_dict: Type[Any]) -> Type[BaseModel]:
    """
        Create a Pydantic model from a TypedDict
    """
    class Model(BaseModel):
        __annotations__ = typed_dict.__annotations__

    return Model


def print_pydantic_models(app):
    for route in app.routes:
        if hasattr(route, "endpoint"):
            print(f"Route: {route.path}")
            for param in route.endpoint.__annotations__.values():
                if issubclass(param, BaseModel):
                    print(f"  Pydantic model: {param.__name__}")
                    for field_name, field_value in param.__annotations__.items():
                        print(f"    Field: {field_name}, Type: {field_value}")


class Orra:
    def __init__(self, state_def=None, **extra: Annotated[
        Any,
        Doc(),
    ]):
        super().__init__(**extra)
        if state_def is None:
            state_def = {}

        self._steps_app = fastapi.FastAPI()
        self._steps = []
        self._StateDict = _create_typed_dict("StateDict", state_def)
        self._StepResponseModel = _create_response_model(self._StateDict)
        self._workflow = StateGraph(self._StateDict)
        self._compiled_workflow = None

    def step(self, func: Callable) -> Callable:
        print(f"decorated with step: {func.__name__}")
        self._register(func)

        response_model = self._StepResponseModel

        @self._steps_app.post(f"/{func.__name__}")
        def wrap_endpoint(v: response_model):
            func(v.dict())

        return func

    # def after(self, act: str) -> Callable:
    #     def decorator(func: Callable) -> Callable:
    #         print(f"decorated {func.__name__} with activity: {act}")
    #         self._workflow = f"{self._workflow} | {func.__name__}"
    #         return func
    #     return decorator

    def steps_server(self) -> Callable:
        self._compiled_workflow = self._compile(self._workflow, self._steps)

        @self._steps_app.post(f"/workflow")
        def wrap_workflow():
            self._compiled_workflow.invoke({})

        return self._steps_app

    def local(self) -> None:
        self._compiled_workflow = self._compile(self._workflow, self._steps)
        self._compiled_workflow.invoke({})

    def _register(self, func: Callable):
        self._workflow.add_node(func.__name__, func)
        self._steps.append(func.__name__)

    @staticmethod
    def _compile(workflow, steps):
        for i in range(len(steps) - 1):
            print(steps[i], steps[i + 1])
            workflow.add_edge(steps[i], steps[i + 1])

        if len(steps) > 1:
            workflow.set_entry_point(steps[0])
            workflow.add_edge(steps[-1], END)

        return workflow.compile()

