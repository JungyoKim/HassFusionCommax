import requests
from lxml import etree
import sys
import time

# --- 설정 (Configuration) ---
ELEVATOR_SERVER_URL = "http://10.0.0.2:29715"
SOAP_ACTION_HEADER = "urn:ces#setEvStatus" # 혹은 SOAP_ACTION_HEADER = "" 로 테스트
                                         # 이전 캡처에서 SOAPAction이 없었으므로 비워두는 것이 더 잘 될 수도 있습니다.
                                         # 만약 안되면 "urn:ces#setEvStatus" 로 시도해보세요.
# -----------------------------

def send_elevator_call(call_direction):
    """
    엘리베이터 호출 요청을 보냅니다.

    Args:
        call_direction (str): 'up' 또는 'down' 중 하나.
    """
    if call_direction not in ['up', 'down']:
        print("오류: call_direction은 'up' 또는 'down'이어야 합니다.")
        return

    call_up_value = 1 if call_direction == 'up' else 0
    call_down_value = 1 if call_direction == 'down' else 0

    soap_body = f"""<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:SOAP-ENC="http://schemas.xmlsoap.org/soap/encoding/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:ns="urn:ces">
<SOAP-ENV:Body SOAP-ENV:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<ns:setEvStatus>
    <in xsi:type="ns:inEvStatus">
        <callUp>{call_up_value}</callUp>
        <callDown>{call_down_value}</callDown>
    </in>
    <isWallpad>1</isWallpad>
</ns:setEvStatus>
</SOAP-ENV:Body>
</SOAP-ENV:Envelope>"""

    headers = {
        "Content-Type": "text/xml; charset=utf-8",
        "SOAPAction": SOAP_ACTION_HEADER
    }

    print(f"\n엘리베이터 {call_direction} 호출 시도 중...")
    try:
        response = requests.post(ELEVATOR_SERVER_URL, headers=headers, data=soap_body)
        response.raise_for_status() # HTTP 오류가 발생하면 예외 발생

        print(f"호출 요청 성공. HTTP 상태 코드: {response.status_code}")
        print("응답 XML:")
        print(response.text)

        # 응답 XML 파싱 (선택 사항: 응답에 따라 성공 여부 추가 확인)
        try:
            root = etree.fromstring(response.text.encode('utf-8'))
            namespaces = {'SOAP-ENV': "http://schemas.xmlsoap.org/soap/envelope/"}
            
            # SOAP Fault 확인
            fault = root.find('.//SOAP-ENV:Fault', namespaces)
            if fault is not None:
                print("\n**SOAP 오류 응답 감지!**")
                for child in fault:
                    print(f"  {child.tag.split('}')[-1]}: {child.text}")
            else:
                print("\n엘리베이터 호출 명령이 서버에 성공적으로 전달되었습니다.")
                print("실제로 엘리베이터가 움직이는지 확인해주세요.")

        except etree.XMLSyntaxError as e:
            print(f"응답 XML 파싱 실패: {e}")
            print("응답 내용이 유효한 XML이 아닐 수 있습니다.")

    except requests.exceptions.RequestException as e:
        print(f"엘리베이터 호출 요청 실패: {e}")
    except Exception as e:
        print(f"알 수 없는 오류 발생: {e}")

if __name__ == "__main__":
    print("--------------------------------------------------")
    print("      엘리베이터 호출 스크립트 by Gemini AI      ")
    print("--------------------------------------------------")
    print("1. 현재 스크립트의 IP 주소는 당신의 동-호수에 매핑될 수 있습니다.")
    print("2. 해당 스크립트를 실행하면 현재 층(IP가 지칭하는 층)으로 엘리베이터가 호출됩니다.")
    print("3. 'up'은 위로가기버튼튼, 'down'은 밑으로가기버튼입니다다.")
    print("--------------------------------------------------")

    while True:
        print("\n어떤 방향으로 엘리베이터를 호출하시겠습니까?")
        direction = input("['up' 또는 'down' 입력, 'q' 입력 시 종료]: ").strip().lower()

        if direction == 'q':
            print("스크립트를 종료합니다.")
            break
        elif direction in ['up', 'down']:
            send_elevator_call(direction)
            time.sleep(1) # 연속 호출 방지
        else:
            print("잘못된 입력입니다. 'up', 'down' 또는 'q'를 입력해주세요.")