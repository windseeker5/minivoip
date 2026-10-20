FROM python:3.12-slim

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py menu.json ./
COPY static/ static/

RUN useradd -r -u 1000 callbox && mkdir -p /data && chown callbox /data
USER callbox

EXPOSE 8000
# One worker on purpose: the ringing call and the open sockets live in its memory. Threads carry the sockets.
CMD ["gunicorn", "-b", "0.0.0.0:8000", "-w", "1", "-k", "gthread", "--threads", "48", "--timeout", "120", "app:app"]
