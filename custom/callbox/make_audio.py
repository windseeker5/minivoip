"""Render the menu prompts in menu.json to MP3s with a neural French-Canadian voice.

Run after changing a prompt:  uv run --with edge-tts python make_audio.py
The guest page falls back to the browser's own voice for any prompt without an MP3.
"""

import asyncio
import json
from pathlib import Path

import edge_tts

VOICE = "fr-CA-SylvieNeural"
BASE = Path(__file__).parent
OUT = BASE / "static" / "appel" / "audio"


async def main():
    prompts = json.loads((BASE / "menu.json").read_text(encoding="utf-8"))["prompts"]
    OUT.mkdir(parents=True, exist_ok=True)
    for key, text in prompts.items():
        await edge_tts.Communicate(text, VOICE, rate="+4%").save(str(OUT / f"{key}.mp3"))
        print(key)


asyncio.run(main())
