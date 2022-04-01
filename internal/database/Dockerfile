FROM cassandra:4

COPY post-hook.sh /post-hook.sh
RUN ["chmod", "+x", "/post-hook.sh"]
ENTRYPOINT ["/post-hook.sh"]
CMD ["cassandra", "-f"]
