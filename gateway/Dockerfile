FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app ./app
COPY routes.yaml .
EXPOSE 8080
# Inside a compose network, point upstreams at service names, e.g.
#   UPSTREAM_USERS_URL=http://user-service:8080
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8080", "--no-access-log"]
