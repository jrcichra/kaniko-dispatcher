FROM python:3.11.3-alpine3.16
WORKDIR /kaniko
COPY . .
RUN pip install -r requirements.txt
ENTRYPOINT ["python", "-u", "/kaniko/action.py"]
