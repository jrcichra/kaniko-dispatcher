FROM python:3.10-alpine3.15
WORKDIR /kaniko
COPY . .
RUN pip install -r requirements.txt
ENTRYPOINT ["python", "-u", "/kaniko/action.py"]