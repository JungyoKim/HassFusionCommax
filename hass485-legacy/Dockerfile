FROM alpine:latest

# 필요한 패키지 설치
RUN apk add --no-cache \
    ca-certificates \
    tzdata

# 작업 디렉토리 생성
WORKDIR /app

# 실행 파일 복사
COPY hass485-linux /app/hass485

# 실행 권한 부여
RUN chmod +x /app/hass485

# 실행 명령
CMD ["/app/hass485"] 