## Lesson: IAM least privilege after refactor

After removing the SES dependency, the role arn:aws:iam::123456789012:role/Deploy
was over-provisioned. The app bundle mx.com.acme.privateapp still referenced it.
