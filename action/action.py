import time
import requests
import json
import os
import sys

NAME = os.environ.get('INPUT_NAME')
URL = os.environ.get('INPUT_URL')
CONTEXT = os.environ.get('INPUT_CONTEXT')
DESTINATION = os.environ.get('INPUT_DESTINATION')
SECRET = os.environ.get('INPUT_SECRET')
ARCH = os.environ.get('INPUT_ARCH')
HEADERS = os.environ.get('INPUT_HEADERS')
EPOCH = time.time_ns()


FULLNAME = f"{NAME}-{EPOCH}"

request = {
    "name": FULLNAME,
    "context": CONTEXT,
    "destination": DESTINATION,
}

if SECRET:
    request["secret"] = SECRET

if ARCH:
    request["arch"] = ARCH

headers = json.loads(HEADERS) if HEADERS else {}

r = requests.post(f"{URL}/kaniko", json=request, headers=headers)
if r.status_code != 200:
    print(r.text)
    sys.exit(1)

JOBNAME = r.json()["name"]

# wait for the job to complete
while True:
    r2 = requests.get(
        f"{URL}/kaniko", headers=headers, params={"name": JOBNAME})
    print(r2.text)
    try:
        response = r2.json()
        if response["done"] and response["pass"]:
            sys.exit(0)
        elif response["done"] and not response["pass"]:
            sys.exit(1)
    except json.decoder.JSONDecodeError:
        print(f"Could not decode response: {r2.text}")
    time.sleep(5)
