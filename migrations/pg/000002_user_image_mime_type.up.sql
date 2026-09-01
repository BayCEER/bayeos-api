ALTER TABLE user_images ADD COLUMN mime_type TEXT NOT NULL DEFAULT 'image/jpeg';

CREATE UNIQUE INDEX uk_user_images_user_id ON user_images (user_id);