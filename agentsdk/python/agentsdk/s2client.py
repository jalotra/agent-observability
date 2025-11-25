import json
from typing import List, Optional
import httpx

from .events import Event


class S2Client:
    def __init__(self, endpoint: str, api_key: str):
        self.endpoint = endpoint.rstrip("/")
        self.api_key = api_key
        self._client = httpx.Client(timeout=10.0)
    
    def _headers(self) -> dict:
        return {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {self.api_key}",
        }
    
    def create_stream(self, stream_name: str) -> None:
        url = f"{self.endpoint}/streams"
        payload = {"stream": stream_name}
        
        response = self._client.post(url, json=payload, headers=self._headers())
        
        if response.status_code == 409:
            return
        
        response.raise_for_status()
    
    def append_event(self, stream_name: str, event: Event) -> None:
        url = f"{self.endpoint}/streams/{stream_name}/records"
        payload = {
            "records": [{"body": event.to_json()}]
        }
        
        response = self._client.post(url, json=payload, headers=self._headers())
        response.raise_for_status()
    
    def append_events(self, stream_name: str, events: List[Event]) -> None:
        url = f"{self.endpoint}/streams/{stream_name}/records"
        payload = {
            "records": [{"body": e.to_json()} for e in events]
        }
        
        response = self._client.post(url, json=payload, headers=self._headers())
        response.raise_for_status()
    
    def close(self) -> None:
        self._client.close()


class StreamReader:
    def __init__(self, client: S2Client, stream_name: str):
        self.client = client
        self.stream_name = stream_name
        self.last_seq = 0
    
    def read_events(self) -> List[Event]:
        url = f"{self.client.endpoint}/streams/{self.stream_name}/records"
        params = {"after": self.last_seq}
        
        response = self.client._client.get(
            url, 
            params=params, 
            headers=self.client._headers()
        )
        response.raise_for_status()
        
        result = response.json()
        events = []
        
        for record in result.get("records", []):
            try:
                event = Event.from_json(record["body"])
                events.append(event)
                if record.get("sequence", 0) > self.last_seq:
                    self.last_seq = record["sequence"]
            except (json.JSONDecodeError, KeyError):
                continue
        
        return events


