package com.auralis.notification.events;

import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Component;
import tools.jackson.databind.ObjectMapper;

/** Feeds the user and content event topics into the EventHandler. */
@Component
public class EventListener {

    public static final String TOPIC_USER_EVENTS = "auralis.user.events";
    public static final String TOPIC_CONTENT_EVENTS = "auralis.content.events";

    private final ObjectMapper json;
    private final EventHandler handler;

    public EventListener(ObjectMapper json, EventHandler handler) {
        this.json = json;
        this.handler = handler;
    }

    @KafkaListener(
            topics = {TOPIC_USER_EVENTS, TOPIC_CONTENT_EVENTS},
            groupId = EventHandler.CONSUMER_GROUP,
            autoStartup = "${NOTIFICATION_RUN_CONSUMER:true}")
    void onMessage(ConsumerRecord<String, String> record) {
        handler.handle(Envelope.parse(json, record.value()));
    }
}
