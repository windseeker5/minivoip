"""Callbox: a "phone call" over the web, with a voice menu and a press-0 transfer to the owner.

The guest's browser plays the menu itself (static/appel/). Only pressing 0 reaches this server: the guest opens a
WebSocket and sends "call", the owner's phones get a Web Push every PUSH_REPEAT_S seconds, the owner answers in the
PWA (static/owner/), and the two browsers talk over WebRTC (relayed by coturn when they can't reach each other).
Nobody answers within RING_TIMEOUT_S: the guest gets a voicemail token and records a message.

One gunicorn worker on purpose: the current call and the open sockets live in memory.
"""

import base64
import hashlib
import hmac
import json
import logging
import os
import secrets
import sqlite3
import threading
import time
from collections import defaultdict, deque
from datetime import timedelta
from pathlib import Path

from flask import Flask, abort, jsonify, request, send_file, send_from_directory, session
from flask_sock import Sock

BASE = Path(__file__).parent
STATIC = BASE / "static"
DATA = Path(os.environ.get("DATA_DIR", "/data"))
VOICEMAIL_DIR = DATA / "voicemail"
DB_PATH = DATA / "callbox.db"
MENU_PATH = Path(os.environ.get("MENU_FILE", BASE / "menu.json"))

PUBLIC_BASE = os.environ.get("PUBLIC_BASE", "https://omt.dresdell.com").rstrip("/")
OWNER_PASSCODE = os.environ.get("OWNER_PASSCODE", "")
VAPID_PUBLIC_KEY = os.environ.get("VAPID_PUBLIC_KEY", "")
VAPID_PRIVATE_KEY = os.environ.get("VAPID_PRIVATE_KEY", "")
VAPID_SUBJECT = os.environ.get("VAPID_SUBJECT", "")
TURN_SECRET = os.environ.get("TURN_SECRET", "")
TURN_HOST = os.environ.get("TURN_HOST", "omt.dresdell.com")
RING_TIMEOUT_S = int(os.environ.get("RING_TIMEOUT_S", "45"))
PUSH_REPEAT_S = int(os.environ.get("PUSH_REPEAT_S", "10"))
VOICEMAIL_MAX_BYTES = 3 * 1024 * 1024

app = Flask(__name__, static_folder=None)
app.config.update(
    SECRET_KEY=os.environ.get("SECRET_KEY") or secrets.token_hex(32),
    SESSION_COOKIE_NAME="callbox",
    SESSION_COOKIE_PATH="/call",
    SESSION_COOKIE_SECURE=PUBLIC_BASE.startswith("https"),
    SESSION_COOKIE_SAMESITE="Lax",
    PERMANENT_SESSION_LIFETIME=timedelta(days=365),
    MAX_CONTENT_LENGTH=VOICEMAIL_MAX_BYTES,
    # Pings keep the socket alive through Caddy and mobile carriers while a call rings.
    SOCK_SERVER_OPTIONS={"ping_interval": 25},
)
sock = Sock(app)
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(message)s", datefmt="%H:%M:%S")
log = logging.getLogger("callbox")


# ---------------------------------------------------------------- storage

def db():
    conn = sqlite3.connect(DB_PATH, timeout=10)
    conn.row_factory = sqlite3.Row
    return conn


def init_db():
    VOICEMAIL_DIR.mkdir(parents=True, exist_ok=True)
    with db() as conn:
        conn.executescript(
            """
            CREATE TABLE IF NOT EXISTS subs (
                endpoint TEXT PRIMARY KEY, info TEXT NOT NULL, ua TEXT, created INTEGER NOT NULL);
            CREATE TABLE IF NOT EXISTS voicemails (
                id TEXT PRIMARY KEY, created INTEGER NOT NULL, mime TEXT NOT NULL, file TEXT NOT NULL,
                size INTEGER NOT NULL, heard INTEGER NOT NULL DEFAULT 0);
            """
        )


init_db()


def load_menu():
    return json.loads(MENU_PATH.read_text(encoding="utf-8"))


# ---------------------------------------------------------------- guards

_hits = defaultdict(deque)
_hits_lock = threading.Lock()


def client_ip():
    # Caddy replaces X-Forwarded-For with the real client address; only Caddy reaches this port.
    return (request.headers.get("X-Forwarded-For") or request.remote_addr or "?").split(",")[0].strip()


def allow(bucket, limit, per_s):
    """Sliding-window rate limit per client IP. True when this hit is allowed."""
    key = (bucket, client_ip())
    now = time.time()
    with _hits_lock:
        q = _hits[key]
        while q and q[0] < now - per_s:
            q.popleft()
        if len(q) >= limit:
            return False
        q.append(now)
        return True


def is_owner():
    return bool(session.get("owner"))


def owner_required():
    if not is_owner():
        abort(401)


@app.before_request
def guard_writes():
    # A custom header can't be sent cross-site without CORS, which this app never grants: that's the CSRF guard.
    if request.method in ("POST", "DELETE") and request.headers.get("X-Call") != "1":
        abort(403)


# ---------------------------------------------------------------- web push

def push_owner(payload):
    """Push to every subscribed owner device, in the background. Gone subscriptions are pruned."""
    if not (VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY and VAPID_SUBJECT):
        app.logger.warning("push skipped: VAPID keys not set")
        return

    def send():
        from pywebpush import WebPushException, webpush

        with db() as conn:
            rows = conn.execute("SELECT endpoint, info FROM subs").fetchall()
        for row in rows:
            started = time.time()
            try:
                resp = webpush(
                    subscription_info=json.loads(row["info"]),
                    data=json.dumps(payload),
                    vapid_private_key=VAPID_PRIVATE_KEY,
                    vapid_claims={"sub": VAPID_SUBJECT},
                    ttl=RING_TIMEOUT_S + 15,
                    headers={"Urgency": "high"},
                    timeout=10,
                )
                log.info("push %s %s -> %s in %d ms", payload.get("kind"), payload.get("n", ""),
                         getattr(resp, "status_code", "?"), (time.time() - started) * 1000)
            except WebPushException as exc:
                status = exc.response.status_code if exc.response is not None else None
                if status in (404, 410):
                    with db() as conn:
                        conn.execute("DELETE FROM subs WHERE endpoint = ?", (row["endpoint"],))
                else:
                    app.logger.warning("push failed (%s): %s", status, exc)
            except Exception as exc:  # network trouble must never break a call
                app.logger.warning("push failed: %s", exc)

    threading.Thread(target=send, daemon=True).start()


# ---------------------------------------------------------------- calls

class Peer:
    """One open WebSocket. Sends are locked: the ring thread and the socket's own thread both write."""

    def __init__(self, ws, role):
        self.ws, self.role = ws, role
        self.lock = threading.Lock()

    def send(self, **msg):
        try:
            with self.lock:
                self.ws.send(json.dumps(msg))
        except Exception:
            pass


class Call:
    def __init__(self, guest):
        self.id = secrets.token_urlsafe(8)
        self.guest = guest
        self.owner = None
        self.state = "ringing"  # ringing -> active -> ended
        self.done = threading.Event()  # set when ringing stops, for any reason


calls_lock = threading.RLock()
current = None  # the one call that's ringing or active
owners = set()  # open owner PWAs
vm_tokens = {}  # voicemail token -> expiry; issued when a call isn't answered


def issue_voicemail_token():
    token = secrets.token_urlsafe(16)
    now = time.time()
    for t, exp in list(vm_tokens.items()):
        if exp < now:
            vm_tokens.pop(t, None)
    vm_tokens[token] = now + 300
    return token


def ring(call):
    """Push the owner every PUSH_REPEAT_S until answered; voicemail after RING_TIMEOUT_S."""
    deadline = time.time() + RING_TIMEOUT_S
    n = 0
    while time.time() < deadline:
        with calls_lock:
            if call.state != "ringing":
                return
        n += 1
        push_owner({"kind": "ring", "call": call.id, "n": n, "title": "📞 Un appel attend",
                    "body": "Touchez pour répondre", "url": f"/call/owner/?call={call.id}"})
        if call.done.wait(min(PUSH_REPEAT_S, max(0, deadline - time.time()))):
            return
    end_unanswered(call, "no-answer")


def end_unanswered(call, why):
    global current
    with calls_lock:
        if call.state != "ringing":
            return
        log.info("call %s not answered (%s)", call.id, why)
        call.state = "ended"
        call.done.set()
        if current is call:
            current = None
        for o in list(owners):
            o.send(type="missed", id=call.id)
    if why == "no-answer":
        call.guest.send(type="no-answer", id=call.id, token=issue_voicemail_token())
    # Replaces the ring notifications (sw.js clears them) and says what happened.
    push_owner({"kind": "missed", "call": call.id, "title": "Appel manqué",
                "body": "Personne n'a répondu. Un message suivra peut-être.", "url": "/call/owner/"})


def end_call(call, by):
    """Hang up an active or ringing call; tell the other side."""
    global current
    with calls_lock:
        if call.state == "ended":
            return
        was_ringing = call.state == "ringing"
        log.info("call %s ended by %s (was %s)", call.id, by.role, call.state)
        call.state = "ended"
        call.done.set()
        if current is call:
            current = None
        if by is not call.guest:
            call.guest.send(type="hangup", id=call.id)
        if call.owner and by is not call.owner:
            call.owner.send(type="hangup", id=call.id)
        if was_ringing:
            for o in list(owners):
                o.send(type="missed", id=call.id)
    if was_ringing and by is call.guest:
        push_owner({"kind": "missed", "call": call.id, "title": "Appel manqué",
                    "body": "Le client a raccroché avant la réponse.", "url": "/call/owner/"})


def same_origin():
    origin = request.headers.get("Origin")
    return origin is None or origin == PUBLIC_BASE or app.debug


@sock.route("/call/ws")
def ws(sock_):
    global current
    role = request.args.get("role")
    if not same_origin() or role not in ("guest", "owner") or (role == "owner" and not is_owner()):
        sock_.close(reason=1008, message="refused")
        return
    me = Peer(sock_, role)
    my_calls = []
    if role == "owner":
        log.info("owner app connected from %s", client_ip())
        with calls_lock:
            owners.add(me)
    try:
        while True:
            raw = sock_.receive()
            if raw is None:
                break
            try:
                msg = json.loads(raw)
            except ValueError:
                continue
            kind = msg.get("type")

            if kind == "diag":
                # Connection reports from both browsers (state, candidate types): the only view into audio trouble.
                log.info("diag %s %s: %s", role, msg.get("id"), json.dumps(msg.get("info"))[:400])
                continue

            if role == "guest" and kind == "call":
                if not allow("call", 20, 600):
                    log.info("call refused: rate limit %s", client_ip())
                    me.send(type="error", reason="rate")
                    continue
                with calls_lock:
                    busy = current is not None
                    if not busy:
                        call = current = Call(me)
                        my_calls.append(call)
                        for o in list(owners):
                            o.send(type="incoming", id=call.id)
                if busy:
                    log.info("call busy %s", client_ip())
                    me.send(type="no-answer", id=None, token=issue_voicemail_token(), busy=True)
                    continue
                log.info("call %s from %s, %d owner app(s) open", call.id, client_ip(), len(owners))
                me.send(type="ringing", id=call.id)
                threading.Thread(target=ring, args=(call,), daemon=True).start()

            elif role == "owner" and kind == "hello":
                with calls_lock:
                    if current and current.state == "ringing":
                        me.send(type="incoming", id=current.id)

            elif role == "owner" and kind in ("accept", "decline"):
                with calls_lock:
                    call = current if current and current.id == msg.get("id") else None
                    if not call or call.state != "ringing":
                        me.send(type="missed", id=msg.get("id"))
                        continue
                    log.info("call %s %s by owner", call.id, kind)
                    if kind == "accept":
                        call.state = "active"
                        call.owner = me
                        call.done.set()
                        my_calls.append(call)
                        for o in list(owners):
                            if o is not me:
                                o.send(type="taken", id=call.id)
                        call.guest.send(type="accepted", id=call.id)
                if kind == "decline":
                    end_unanswered(call, "no-answer")

            elif kind in ("offer", "answer", "ice"):
                with calls_lock:
                    call = current if current and current.id == msg.get("id") else None
                    other = None
                    if call and call.state == "active":
                        other = call.owner if me is call.guest else call.guest if me is call.owner else None
                if other:
                    other.send(**{k: msg[k] for k in ("type", "id", "sdp", "candidate") if k in msg})

            elif kind == "hangup":
                with calls_lock:
                    call = current if current and current.id == msg.get("id") else None
                if call and me in (call.guest, call.owner):
                    end_call(call, me)
    finally:
        with calls_lock:
            owners.discard(me)
        for call in my_calls:
            end_call(call, me)


# ---------------------------------------------------------------- api

@app.get("/call/api/turn")
def turn():
    if not allow("turn", 30, 600):
        abort(429)
    expiry = int(time.time()) + 2 * 3600  # long enough for a full call; coturn rechecks it on each refresh
    username = f"{expiry}:{'owner' if is_owner() else 'guest'}"
    password = base64.b64encode(hmac.new(TURN_SECRET.encode(), username.encode(), hashlib.sha1).digest()).decode()
    return jsonify(iceServers=[
        {"urls": f"stun:{TURN_HOST}:3478"},
        {"urls": [f"turn:{TURN_HOST}:3478?transport=udp", f"turn:{TURN_HOST}:3478?transport=tcp"],
         "username": username, "credential": password},
    ])


@app.get("/call/api/menu")
def menu():
    return jsonify(load_menu())


@app.post("/call/api/login")
def login():
    if not allow("login", 6, 600):
        abort(429)
    code = (request.get_json(silent=True) or {}).get("passcode", "")
    if not OWNER_PASSCODE or not hmac.compare_digest(code.encode(), OWNER_PASSCODE.encode()):
        return jsonify(ok=False), 401
    session.permanent = True
    session["owner"] = True
    return jsonify(ok=True)


@app.post("/call/api/logout")
def logout():
    session.clear()
    return jsonify(ok=True)


@app.get("/call/api/me")
def me():
    return jsonify(owner=is_owner(), vapid=VAPID_PUBLIC_KEY)


@app.post("/call/api/push/subscribe")
def subscribe():
    owner_required()
    info = request.get_json(silent=True) or {}
    if not info.get("endpoint") or not info.get("keys"):
        abort(400)
    with db() as conn:
        conn.execute(
            "INSERT OR REPLACE INTO subs (endpoint, info, ua, created) VALUES (?, ?, ?, ?)",
            (info["endpoint"], json.dumps(info), request.headers.get("User-Agent", "")[:200], int(time.time())),
        )
    return jsonify(ok=True)


@app.post("/call/api/push/test")
def push_test():
    owner_required()
    push_owner({"kind": "test", "call": "test", "title": "📞 Test de sonnerie",
                "body": "Les notifications fonctionnent.", "url": "/call/owner/"})
    with db() as conn:
        count = conn.execute("SELECT COUNT(*) FROM subs").fetchone()[0]
    return jsonify(ok=True, devices=count)


@app.post("/call/api/voicemail")
def voicemail_upload():
    token = request.form.get("token", "")
    if vm_tokens.pop(token, 0) < time.time():
        abort(403)
    blob = request.files.get("audio")
    if not blob:
        abort(400)
    mime = (blob.mimetype or "audio/webm").split(";")[0]
    if mime not in ("audio/webm", "audio/mp4", "audio/ogg", "audio/aac", "audio/mpeg"):
        abort(415)
    vid = secrets.token_hex(8)
    ext = {"audio/webm": "webm", "audio/mp4": "m4a", "audio/ogg": "ogg", "audio/aac": "aac", "audio/mpeg": "mp3"}[mime]
    path = VOICEMAIL_DIR / f"{vid}.{ext}"
    blob.save(path)
    with db() as conn:
        conn.execute("INSERT INTO voicemails (id, created, mime, file, size) VALUES (?, ?, ?, ?, ?)",
                     (vid, int(time.time()), mime, path.name, path.stat().st_size))
    push_owner({"kind": "voicemail", "call": vid, "title": "Nouveau message vocal",
                "body": "Touchez pour l'écouter.", "url": f"/call/owner/?vm={vid}"})
    return jsonify(ok=True)


@app.get("/call/api/voicemails")
def voicemail_list():
    owner_required()
    with db() as conn:
        rows = conn.execute("SELECT id, created, mime, size, heard FROM voicemails ORDER BY created DESC LIMIT 50")
        return jsonify([dict(r) for r in rows])


@app.get("/call/api/voicemail/<vid>")
def voicemail_get(vid):
    owner_required()
    with db() as conn:
        row = conn.execute("SELECT file, mime FROM voicemails WHERE id = ?", (vid,)).fetchone()
        if not row:
            abort(404)
        conn.execute("UPDATE voicemails SET heard = 1 WHERE id = ?", (vid,))
    return send_file(VOICEMAIL_DIR / row["file"], mimetype=row["mime"], conditional=True)


@app.delete("/call/api/voicemail/<vid>")
def voicemail_delete(vid):
    owner_required()
    with db() as conn:
        row = conn.execute("SELECT file FROM voicemails WHERE id = ?", (vid,)).fetchone()
        if row:
            (VOICEMAIL_DIR / row["file"]).unlink(missing_ok=True)
            conn.execute("DELETE FROM voicemails WHERE id = ?", (vid,))
    return jsonify(ok=True)


# ---------------------------------------------------------------- pages

@app.get("/appel/")
def guest_page():
    return send_from_directory(STATIC / "appel", "index.html")


@app.get("/appel")
def guest_page_bare():
    return guest_page()


@app.get("/appel/<path:name>")
def guest_static(name):
    return send_from_directory(STATIC / "appel", name, max_age=300)


@app.get("/call/owner/")
def owner_page():
    return send_from_directory(STATIC / "owner", "index.html", max_age=0)


@app.get("/call/owner/<path:name>")
def owner_static(name):
    resp = send_from_directory(STATIC / "owner", name, max_age=0 if name in ("sw.js", "manifest.webmanifest") else 300)
    if name == "sw.js":
        resp.headers["Service-Worker-Allowed"] = "/call/owner/"
    return resp


@app.after_request
def headers(resp):
    resp.headers.setdefault("X-Content-Type-Options", "nosniff")
    resp.headers.setdefault("Referrer-Policy", "same-origin")
    return resp
