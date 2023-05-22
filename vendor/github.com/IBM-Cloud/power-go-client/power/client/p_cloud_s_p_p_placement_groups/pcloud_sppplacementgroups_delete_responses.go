// Code generated by go-swagger; DO NOT EDIT.

package p_cloud_s_p_p_placement_groups

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"github.com/IBM-Cloud/power-go-client/power/models"
)

// PcloudSppplacementgroupsDeleteReader is a Reader for the PcloudSppplacementgroupsDelete structure.
type PcloudSppplacementgroupsDeleteReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PcloudSppplacementgroupsDeleteReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewPcloudSppplacementgroupsDeleteOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	case 400:
		result := NewPcloudSppplacementgroupsDeleteBadRequest()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	case 401:
		result := NewPcloudSppplacementgroupsDeleteUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	case 403:
		result := NewPcloudSppplacementgroupsDeleteForbidden()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	case 404:
		result := NewPcloudSppplacementgroupsDeleteNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	case 409:
		result := NewPcloudSppplacementgroupsDeleteConflict()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	case 500:
		result := NewPcloudSppplacementgroupsDeleteInternalServerError()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result
	default:
		return nil, runtime.NewAPIError("response status code does not match any response statuses defined for this endpoint in the swagger spec", response, response.Code())
	}
}

// NewPcloudSppplacementgroupsDeleteOK creates a PcloudSppplacementgroupsDeleteOK with default headers values
func NewPcloudSppplacementgroupsDeleteOK() *PcloudSppplacementgroupsDeleteOK {
	return &PcloudSppplacementgroupsDeleteOK{}
}

/*
PcloudSppplacementgroupsDeleteOK describes a response with status code 200, with default header values.

OK
*/
type PcloudSppplacementgroupsDeleteOK struct {
	Payload models.Object
}

// IsSuccess returns true when this pcloud sppplacementgroups delete o k response has a 2xx status code
func (o *PcloudSppplacementgroupsDeleteOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this pcloud sppplacementgroups delete o k response has a 3xx status code
func (o *PcloudSppplacementgroupsDeleteOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this pcloud sppplacementgroups delete o k response has a 4xx status code
func (o *PcloudSppplacementgroupsDeleteOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this pcloud sppplacementgroups delete o k response has a 5xx status code
func (o *PcloudSppplacementgroupsDeleteOK) IsServerError() bool {
	return false
}

// IsCode returns true when this pcloud sppplacementgroups delete o k response a status code equal to that given
func (o *PcloudSppplacementgroupsDeleteOK) IsCode(code int) bool {
	return code == 200
}

func (o *PcloudSppplacementgroupsDeleteOK) Error() string {
	return fmt.Sprintf("[DELETE /pcloud/v1/cloud-instances/{cloud_instance_id}/spp-placement-groups/{spp_placement_group_id}][%d] pcloudSppplacementgroupsDeleteOK  %+v", 200, o.Payload)
}

func (o *PcloudSppplacementgroupsDeleteOK) String() string {
	return fmt.Sprintf("[DELETE /pcloud/v1/cloud-instances/{cloud_instance_id}/spp-placement-groups/{spp_placement_group_id}][%d] pcloudSppplacementgroupsDeleteOK  %+v", 200, o.Payload)
}

func (o *PcloudSppplacementgroupsDeleteOK) GetPayload() models.Object {
	return o.Payload
}

func (o *PcloudSppplacementgroupsDeleteOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	// response payload
	if err := consumer.Consume(response.Body(), &o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewPcloudSppplacementgroupsDeleteBadRequest creates a PcloudSppplacementgroupsDeleteBadRequest with default headers values
func NewPcloudSppplacementgroupsDeleteBadRequest() *PcloudSppplacementgroupsDeleteBadRequest {
	return &PcloudSppplacementgroupsDeleteBadRequest{}
}

/*
PcloudSppplacementgroupsDeleteBadRequest describes a response with status code 400, with default header values.

Bad Request
*/
type PcloudSppplacementgroupsDeleteBadRequest struct {
	Payload *models.Error
}

// IsSuccess returns true when this pcloud sppplacementgroups delete bad request response has a 2xx status code
func (o *PcloudSppplacementgroupsDeleteBadRequest) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this pcloud sppplacementgroups delete bad request response has a 3xx status code
func (o *PcloudSppplacementgroupsDeleteBadRequest) IsRedirect() bool {
	return false
}

// IsClientError returns true when this pcloud sppplacementgroups delete bad request response has a 4xx status code
func (o *PcloudSppplacementgroupsDeleteBadRequest) IsClientError() bool {
	return true
}

// IsServerError returns true when this pcloud sppplacementgroups delete bad request response has a 5xx status code
func (o *PcloudSppplacementgroupsDeleteBadRequest) IsServerError() bool {
	return false
}

// IsCode returns true when this pcloud sppplacementgroups delete bad request response a status code equal to that given
func (o *PcloudSppplacementgroupsDeleteBadRequest) IsCode(code int) bool {
	return code == 400
}

func (o *PcloudSppplacementgroupsDeleteBadRequest) Error() string {
	return fmt.Sprintf("[DELETE /pcloud/v1/cloud-instances/{cloud_instance_id}/spp-placement-groups/{spp_placement_group_id}][%d] pcloudSppplacementgroupsDeleteBadRequest  %+v", 400, o.Payload)
}

func (o *PcloudSppplacementgroupsDeleteBadRequest) String() string {
	return fmt.Sprintf("[DELETE /pcloud/v1/cloud-instances/{cloud_instance_id}/spp-placement-groups/{spp_placement_group_id}][%d] pcloudSppplacementgroupsDeleteBadRequest  %+v", 400, o.Payload)
}

func (o *PcloudSppplacementgroupsDeleteBadRequest) GetPayload() *models.Error {
	return o.Payload
}

func (o *PcloudSppplacementgroupsDeleteBadRequest) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Error)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewPcloudSppplacementgroupsDeleteUnauthorized creates a PcloudSppplacementgroupsDeleteUnauthorized with default headers values
func NewPcloudSppplacementgroupsDeleteUnauthorized() *PcloudSppplacementgroupsDeleteUnauthorized {
	return &PcloudSppplacementgroupsDeleteUnauthorized{}
}

/*
PcloudSppplacementgroupsDeleteUnauthorized describes a response with status code 401, with default header values.

Unauthorized
*/
type PcloudSppplacementgroupsDeleteUnauthorized struct {
	Payload *models.Error
}

// IsSuccess returns true when this pcloud sppplacementgroups delete unauthorized response has a 2xx status code
func (o *PcloudSppplacementgroupsDeleteUnauthorized) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this pcloud sppplacementgroups delete unauthorized response has a 3xx status code
func (o *PcloudSppplacementgroupsDeleteUnauthorized) IsRedirect() bool {
	return false
}

// IsClientError returns true when this pcloud sppplacementgroups delete unauthorized response has a 4xx status code
func (o *PcloudSppplacementgroupsDeleteUnauthorized) IsClientError() bool {
	return true
}

// IsServerError returns true when this pcloud sppplacementgroups delete unauthorized response has a 5xx status code
func (o *PcloudSppplacementgroupsDeleteUnauthorized) IsServerError() bool {
	return false
}

// IsCode returns true when this pcloud sppplacementgroups delete unauthorized response a status code equal to that given
func (o *PcloudSppplacementgroupsDeleteUnauthorized) IsCode(code int) bool {
	return code == 401
}

func (o *PcloudSppplacementgroupsDeleteUnauthorized) Error() string {
	return fmt.Sprintf("[DELETE /pcloud/v1/cloud-instances/{cloud_instance_id}/spp-placement-groups/{spp_placement_group_id}][%d] pcloudSppplacementgroupsDeleteUnauthorized  %+v", 401, o.Payload)
}

func (o *PcloudSppplacementgroupsDeleteUnauthorized) String() string {
	return fmt.Sprintf("[DELETE /pcloud/v1/cloud-instances/{cloud_instance_id}/spp-placement-groups/{spp_placement_group_id}][%d] pcloudSppplacementgroupsDeleteUnauthorized  %+v", 401, o.Payload)
}

func (o *PcloudSppplacementgroupsDeleteUnauthorized) GetPayload() *models.Error {
	return o.Payload
}

func (o *PcloudSppplacementgroupsDeleteUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Error)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewPcloudSppplacementgroupsDeleteForbidden creates a PcloudSppplacementgroupsDeleteForbidden with default headers values
func NewPcloudSppplacementgroupsDeleteForbidden() *PcloudSppplacementgroupsDeleteForbidden {
	return &PcloudSppplacementgroupsDeleteForbidden{}
}

/*
PcloudSppplacementgroupsDeleteForbidden describes a response with status code 403, with default header values.

Forbidden
*/
type PcloudSppplacementgroupsDeleteForbidden struct {
	Payload *models.Error
}

// IsSuccess returns true when this pcloud sppplacementgroups delete forbidden response has a 2xx status code
func (o *PcloudSppplacementgroupsDeleteForbidden) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this pcloud sppplacementgroups delete forbidden response has a 3xx status code
func (o *PcloudSppplacementgroupsDeleteForbidden) IsRedirect() bool {
	return false
}

// IsClientError returns true when this pcloud sppplacementgroups delete forbidden response has a 4xx status code
func (o *PcloudSppplacementgroupsDeleteForbidden) IsClientError() bool {
	return true
}

// IsServerError returns true when this pcloud sppplacementgroups delete forbidden response has a 5xx status code
func (o *PcloudSppplacementgroupsDeleteForbidden) IsServerError() bool {
	return false
}

// IsCode returns true when this pcloud sppplacementgroups delete forbidden response a status code equal to that given
func (o *PcloudSppplacementgroupsDeleteForbidden) IsCode(code int) bool {
	return code == 403
}

func (o *PcloudSppplacementgroupsDeleteForbidden) Error() string {
	return fmt.Sprintf("[DELETE /pcloud/v1/cloud-instances/{cloud_instance_id}/spp-placement-groups/{spp_placement_group_id}][%d] pcloudSppplacementgroupsDeleteForbidden  %+v", 403, o.Payload)
}

func (o *PcloudSppplacementgroupsDeleteForbidden) String() string {
	return fmt.Sprintf("[DELETE /pcloud/v1/cloud-instances/{cloud_instance_id}/spp-placement-groups/{spp_placement_group_id}][%d] pcloudSppplacementgroupsDeleteForbidden  %+v", 403, o.Payload)
}

func (o *PcloudSppplacementgroupsDeleteForbidden) GetPayload() *models.Error {
	return o.Payload
}

func (o *PcloudSppplacementgroupsDeleteForbidden) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Error)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewPcloudSppplacementgroupsDeleteNotFound creates a PcloudSppplacementgroupsDeleteNotFound with default headers values
func NewPcloudSppplacementgroupsDeleteNotFound() *PcloudSppplacementgroupsDeleteNotFound {
	return &PcloudSppplacementgroupsDeleteNotFound{}
}

/*
PcloudSppplacementgroupsDeleteNotFound describes a response with status code 404, with default header values.

Not Found
*/
type PcloudSppplacementgroupsDeleteNotFound struct {
	Payload *models.Error
}

// IsSuccess returns true when this pcloud sppplacementgroups delete not found response has a 2xx status code
func (o *PcloudSppplacementgroupsDeleteNotFound) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this pcloud sppplacementgroups delete not found response has a 3xx status code
func (o *PcloudSppplacementgroupsDeleteNotFound) IsRedirect() bool {
	return false
}

// IsClientError returns true when this pcloud sppplacementgroups delete not found response has a 4xx status code
func (o *PcloudSppplacementgroupsDeleteNotFound) IsClientError() bool {
	return true
}

// IsServerError returns true when this pcloud sppplacementgroups delete not found response has a 5xx status code
func (o *PcloudSppplacementgroupsDeleteNotFound) IsServerError() bool {
	return false
}

// IsCode returns true when this pcloud sppplacementgroups delete not found response a status code equal to that given
func (o *PcloudSppplacementgroupsDeleteNotFound) IsCode(code int) bool {
	return code == 404
}

func (o *PcloudSppplacementgroupsDeleteNotFound) Error() string {
	return fmt.Sprintf("[DELETE /pcloud/v1/cloud-instances/{cloud_instance_id}/spp-placement-groups/{spp_placement_group_id}][%d] pcloudSppplacementgroupsDeleteNotFound  %+v", 404, o.Payload)
}

func (o *PcloudSppplacementgroupsDeleteNotFound) String() string {
	return fmt.Sprintf("[DELETE /pcloud/v1/cloud-instances/{cloud_instance_id}/spp-placement-groups/{spp_placement_group_id}][%d] pcloudSppplacementgroupsDeleteNotFound  %+v", 404, o.Payload)
}

func (o *PcloudSppplacementgroupsDeleteNotFound) GetPayload() *models.Error {
	return o.Payload
}

func (o *PcloudSppplacementgroupsDeleteNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Error)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewPcloudSppplacementgroupsDeleteConflict creates a PcloudSppplacementgroupsDeleteConflict with default headers values
func NewPcloudSppplacementgroupsDeleteConflict() *PcloudSppplacementgroupsDeleteConflict {
	return &PcloudSppplacementgroupsDeleteConflict{}
}

/*
PcloudSppplacementgroupsDeleteConflict describes a response with status code 409, with default header values.

Conflict
*/
type PcloudSppplacementgroupsDeleteConflict struct {
	Payload *models.Error
}

// IsSuccess returns true when this pcloud sppplacementgroups delete conflict response has a 2xx status code
func (o *PcloudSppplacementgroupsDeleteConflict) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this pcloud sppplacementgroups delete conflict response has a 3xx status code
func (o *PcloudSppplacementgroupsDeleteConflict) IsRedirect() bool {
	return false
}

// IsClientError returns true when this pcloud sppplacementgroups delete conflict response has a 4xx status code
func (o *PcloudSppplacementgroupsDeleteConflict) IsClientError() bool {
	return true
}

// IsServerError returns true when this pcloud sppplacementgroups delete conflict response has a 5xx status code
func (o *PcloudSppplacementgroupsDeleteConflict) IsServerError() bool {
	return false
}

// IsCode returns true when this pcloud sppplacementgroups delete conflict response a status code equal to that given
func (o *PcloudSppplacementgroupsDeleteConflict) IsCode(code int) bool {
	return code == 409
}

func (o *PcloudSppplacementgroupsDeleteConflict) Error() string {
	return fmt.Sprintf("[DELETE /pcloud/v1/cloud-instances/{cloud_instance_id}/spp-placement-groups/{spp_placement_group_id}][%d] pcloudSppplacementgroupsDeleteConflict  %+v", 409, o.Payload)
}

func (o *PcloudSppplacementgroupsDeleteConflict) String() string {
	return fmt.Sprintf("[DELETE /pcloud/v1/cloud-instances/{cloud_instance_id}/spp-placement-groups/{spp_placement_group_id}][%d] pcloudSppplacementgroupsDeleteConflict  %+v", 409, o.Payload)
}

func (o *PcloudSppplacementgroupsDeleteConflict) GetPayload() *models.Error {
	return o.Payload
}

func (o *PcloudSppplacementgroupsDeleteConflict) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Error)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewPcloudSppplacementgroupsDeleteInternalServerError creates a PcloudSppplacementgroupsDeleteInternalServerError with default headers values
func NewPcloudSppplacementgroupsDeleteInternalServerError() *PcloudSppplacementgroupsDeleteInternalServerError {
	return &PcloudSppplacementgroupsDeleteInternalServerError{}
}

/*
PcloudSppplacementgroupsDeleteInternalServerError describes a response with status code 500, with default header values.

Internal Server Error
*/
type PcloudSppplacementgroupsDeleteInternalServerError struct {
	Payload *models.Error
}

// IsSuccess returns true when this pcloud sppplacementgroups delete internal server error response has a 2xx status code
func (o *PcloudSppplacementgroupsDeleteInternalServerError) IsSuccess() bool {
	return false
}

// IsRedirect returns true when this pcloud sppplacementgroups delete internal server error response has a 3xx status code
func (o *PcloudSppplacementgroupsDeleteInternalServerError) IsRedirect() bool {
	return false
}

// IsClientError returns true when this pcloud sppplacementgroups delete internal server error response has a 4xx status code
func (o *PcloudSppplacementgroupsDeleteInternalServerError) IsClientError() bool {
	return false
}

// IsServerError returns true when this pcloud sppplacementgroups delete internal server error response has a 5xx status code
func (o *PcloudSppplacementgroupsDeleteInternalServerError) IsServerError() bool {
	return true
}

// IsCode returns true when this pcloud sppplacementgroups delete internal server error response a status code equal to that given
func (o *PcloudSppplacementgroupsDeleteInternalServerError) IsCode(code int) bool {
	return code == 500
}

func (o *PcloudSppplacementgroupsDeleteInternalServerError) Error() string {
	return fmt.Sprintf("[DELETE /pcloud/v1/cloud-instances/{cloud_instance_id}/spp-placement-groups/{spp_placement_group_id}][%d] pcloudSppplacementgroupsDeleteInternalServerError  %+v", 500, o.Payload)
}

func (o *PcloudSppplacementgroupsDeleteInternalServerError) String() string {
	return fmt.Sprintf("[DELETE /pcloud/v1/cloud-instances/{cloud_instance_id}/spp-placement-groups/{spp_placement_group_id}][%d] pcloudSppplacementgroupsDeleteInternalServerError  %+v", 500, o.Payload)
}

func (o *PcloudSppplacementgroupsDeleteInternalServerError) GetPayload() *models.Error {
	return o.Payload
}

func (o *PcloudSppplacementgroupsDeleteInternalServerError) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.Error)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
