import requests

# 요청 URL
url = "http://10.9.1.37:29752/"

# SOAP XML 본문
soap_body = """
<v:Envelope xmlns:i="http://www.w3.org/2001/XMLSchema-instance" xmlns:d="http://www.w3.org/2001/XMLSchema" xmlns:c="http://schemas.xmlsoap.org/soap/encoding/" xmlns:v="http://schemas.xmlsoap.org/soap/envelope/"><v:Header /><v:Body><n0:setOutOfBandDoorOpen id="o0" c:root="1" xmlns:n0="urn:clbs"><in i:type="d:int">15</in></n0:setOutOfBandDoorOpen></v:Body></v:Envelope>
"""

# 요청 헤더
headers = {
    "User-Agent": "kSOAP/2.0",
    "SOAPAction": "",  # SOAPAction이 비어 있음
    "Content-Type": "text/xml",
    "Connection": "close",
    "Content-Length": str(len(soap_body.encode('utf-8'))), # 바디 길이를 정확히 계산하여 설정
    "Accept": "*, */*",
    "Host": "10.9.1.37:29752"
}

print("보낼 요청 본문:")
print(soap_body)
print("\n")

try:
    # POST 요청 보내기
    response = requests.post(url, headers=headers, data=soap_body)

    # 응답 정보 출력
    print(f"응답 상태 코드: {response.status_code}")
    print("응답 헤더:")
    for header, value in response.headers.items():
        print(f"  {header}: {value}")
    print("\n응답 본문:")
    print(response.text)

    # 응답 본문에서 'out' 값 추출 (간단한 XML 파싱)
    if "<out>" in response.text and "</out>" in response.text:
        start = response.text.find("<out>") + len("<out>")
        end = response.text.find("</out>")
        out_value = response.text[start:end]
        print(f"\n추출된 'out' 값: {out_value}")

except requests.exceptions.RequestException as e:
    print(f"요청 중 오류 발생: {e}")