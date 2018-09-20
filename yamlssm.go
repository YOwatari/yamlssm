package yamlssm

import (
	"reflect"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
)

// ssmDecrypter stores the AWS Session used for SSM decrypter.
type ssmDecrypter struct {
	sess *session.Session
	svc  ssmiface.SSMAPI
}

// expand returns to decrypt SSM parameter value.
func (d *ssmDecrypter) expand(encrypted string) (string, error) {
	trimed := strings.TrimPrefix(encrypted, "ssm://")

	params := &ssm.GetParameterInput{
		Name:           aws.String(trimed),
		WithDecryption: aws.Bool(true),
	}
	resp, err := d.svc.GetParameter(params)
	if err != nil {
		return "", err
	}
	return *resp.Parameter.Value, nil
}

// override decrypt and override the "ssm://" cipher.
func (d *ssmDecrypter) override(out interface{}) error {
	v := reflect.ValueOf(out)

	if !v.IsValid() {
		return nil
	}

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	copy := reflect.New(v.Type()).Elem()

	d.decryptCopyRecursive(copy, v)
	v.Set(copy)
	return nil
}

// decryptCopyRecursive decrypts ssm and does actual copying of the interface.
func (d *ssmDecrypter) decryptCopyRecursive(copy, original reflect.Value) {
	switch original.Kind() {
	case reflect.Interface:
		if original.IsNil() {
			return
		}

		originalValue := original.Elem()
		copyValue := reflect.New(originalValue.Type()).Elem()

		d.decryptCopyRecursive(copyValue, originalValue)
		copy.Set(copyValue)

	case reflect.Ptr:
		originalValue := original.Elem()
		if !originalValue.IsValid() {
			return
		}
		copy.Set(reflect.New(originalValue.Type()))
		d.decryptCopyRecursive(copy.Elem(), originalValue)

	case reflect.Struct:
		for i := 0; i < original.NumField(); i++ {
			d.decryptCopyRecursive(copy.Field(i), original.Field(i))
		}

	case reflect.Slice:
		copy.Set(reflect.MakeSlice(original.Type(), original.Len(), original.Cap()))
		for i := 0; i < original.Len(); i++ {
			d.decryptCopyRecursive(copy.Index(i), original.Index(i))
		}

	case reflect.Map:
		copy.Set(reflect.MakeMap(original.Type()))

		for _, key := range original.MapKeys() {
			originalValue := original.MapIndex(key)
			copyValue := reflect.New(originalValue.Type()).Elem()

			d.decryptCopyRecursive(copyValue, originalValue)
			copy.SetMapIndex(key, copyValue)
		}

	case reflect.String:
		if copy.CanSet() {
			copy.SetString(d.decrypt(original.Interface().(string)))
		}

	default:
		copy.Set(original)
	}
}

// deccrypt decrypts string begins with "ssm://".
func (d *ssmDecrypter) decrypt(s string) string {
	if strings.HasPrefix(s, "ssm://") {
		actual, _ := d.expand(s)
		return actual
	}
	return s
}

// newssmDecrypter returns a new ssmDecrypter.
func newssmDecrypter() *ssmDecrypter {
	sess, _ := session.NewSession()
	svc := ssm.New(sess)
	return &ssmDecrypter{sess, svc}
}

// Unmarshal works same as gopkg.in/yaml.v2.
//
// After unmarshal YAML files, yamlssm replace value prefixed `ssm://`
// by encrypted value which stored in your System Manager Parameter Store.
func Unmarshal(in []byte, out interface{}) error {
	if err := yaml.Unmarshal(in, out); err != nil {
		return err
	}
	d := newssmDecrypter()
	return d.override(out)
}
