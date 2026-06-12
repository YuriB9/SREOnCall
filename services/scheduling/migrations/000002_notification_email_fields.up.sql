ALTER TABLE scheduling.tenant_notification_config
    ADD COLUMN mattermost_enabled   boolean NOT NULL DEFAULT true,
    ADD COLUMN email_enabled        boolean NOT NULL DEFAULT true,
    ADD COLUMN email_reply_to       text    NOT NULL DEFAULT '',
    ADD COLUMN email_subject_prefix text    NOT NULL DEFAULT '';
