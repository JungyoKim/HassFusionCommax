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
    print(f"서버 URL: {ELEVATOR_SERVER_URL}")
    print(f"요청 헤더: {headers}")
    print(f"SOAP Body 길이: {len(soap_body)} 문자")
    
    try:
        response = requests.post(ELEVATOR_SERVER_URL, headers=headers, data=soap_body, timeout=10)
        
        # HTTP 상태 코드 상세 분석
        print(f"\n=== HTTP 응답 상세 정보 ===")
        print(f"HTTP 상태 코드: {response.status_code}")
        print(f"HTTP 상태 메시지: {response.reason}")
        print(f"응답 헤더:")
        for key, value in response.headers.items():
            print(f"  {key}: {value}")
        
        # 응답 내용 출력
        print(f"\n=== 응답 내용 ===")
        print(f"응답 텍스트 길이: {len(response.text)} 문자")
        print(f"응답 내용:")
        print("-" * 50)
        print(response.text)
        print("-" * 50)
        
        # HTTP 오류 상태 코드 처리
        if response.status_code >= 400:
            print(f"\n⚠️  HTTP 오류 발생!")
            print(f"오류 코드: {response.status_code}")
            print(f"오류 메시지: {response.reason}")
            
            if response.status_code == 500:
                print("서버 내부 오류 (500) - 서버 측 문제일 가능성이 높습니다.")
            elif response.status_code == 404:
                print("리소스를 찾을 수 없음 (404) - URL이 올바른지 확인하세요.")
            elif response.status_code == 403:
                print("접근 거부 (403) - 권한이 없거나 인증이 필요합니다.")
            elif response.status_code == 400:
                print("잘못된 요청 (400) - 요청 형식이 올바르지 않습니다.")
            
            # 오류 응답도 XML 파싱 시도
            if response.text.strip():
                try:
                    root = etree.fromstring(response.text.encode('utf-8'))
                    print("\n오류 응답 XML 파싱 결과:")
                    print(etree.tostring(root, pretty_print=True, encoding='unicode'))
                except etree.XMLSyntaxError as e:
                    print(f"오류 응답 XML 파싱 실패: {e}")
                    print("응답이 유효한 XML이 아닙니다.")
            
            return  # 오류 발생 시 함수 종료
        
        # 성공적인 응답 처리
        print(f"\n✅ HTTP 요청 성공!")
        
        # 응답 XML 파싱 (선택 사항: 응답에 따라 성공 여부 추가 확인)
        if response.text.strip():
            try:
                root = etree.fromstring(response.text.encode('utf-8'))
                namespaces = {'SOAP-ENV': "http://schemas.xmlsoap.org/soap/envelope/"}
                
                # SOAP Fault 확인
                fault = root.find('.//SOAP-ENV:Fault', namespaces)
                if fault is not None:
                    print("\n⚠️  SOAP 오류 응답 감지!")
                    print("SOAP Fault 상세 정보:")
                    for child in fault:
                        tag_name = child.tag.split('}')[-1] if '}' in child.tag else child.tag
                        print(f"  {tag_name}: {child.text}")
                else:
                    print("\n✅ 엘리베이터 호출 명령이 서버에 성공적으로 전달되었습니다.")
                    print("실제로 엘리베이터가 움직이는지 확인해주세요.")

            except etree.XMLSyntaxError as e:
                print(f"응답 XML 파싱 실패: {e}")
                print("응답 내용이 유효한 XML이 아닐 수 있습니다.")
        else:
            print("\n⚠️  서버에서 빈 응답을 받았습니다.")

    except requests.exceptions.Timeout:
        print(f"\n⏰ 요청 시간 초과 (10초)")
        print("서버가 응답하지 않거나 네트워크 연결이 느립니다.")
    except requests.exceptions.ConnectionError as e:
        print(f"\n🔌 연결 오류: {e}")
        print("서버에 연결할 수 없습니다. 다음을 확인하세요:")
        print("  - 서버가 실행 중인지")
        print("  - IP 주소와 포트가 올바른지")
        print("  - 네트워크 연결이 정상인지")
    except requests.exceptions.RequestException as e:
        print(f"\n❌ 요청 오류: {e}")
        print(f"오류 유형: {type(e).__name__}")
    except Exception as e:
        print(f"\n💥 알 수 없는 오류 발생: {e}")
        print(f"오류 유형: {type(e).__name__}")
        import traceback
        print("상세 오류 정보:")
        traceback.print_exc()

if __name__ == "__main__":
    print("--------------------------------------------------")
    print("               엘리베이터 호출 스크립트               ")
    print("--------------------------------------------------")
    print("1. 현재 스크립트의 IP 주소는 당신의 동-호수에 매핑될 수 있습니다.")
    print("2. 해당 스크립트를 실행하면 현재 층(IP가 지칭하는 층)으로 엘리베이터가 호출됩니다.")
    print("3. 'up'은 위로가기버튼, 'down'은 밑으로가기버튼입니다다.")
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