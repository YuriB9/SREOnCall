ALTER TABLE scheduling.tenant_notification_config
    DROP COLUMN mattermost_enabled,
    DROP COLUMN email_enabled,
    DROP COLUMN email_reply_to,
    DROP COLUMN email_subject_prefix;
